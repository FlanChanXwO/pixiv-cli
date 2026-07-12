package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenResourceForwardsConditionsAndReturnsStreaming206(t *testing.T) {
	var gotHeaders http.Header
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.Header().Add("Content-Type", "image/jpeg")
		w.Header().Add("ETag", `"first"`)
		w.Header().Add("ETag", `"second"`)
		w.Header().Set("Content-Range", "bytes 2-12/13")
		w.Header().Add("Set-Cookie", "secret=cookie")
		w.Header().Add("X-Internal", "hidden")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "stream-body")
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "secret"}})
	httpClient := server.Client()
	httpClient.Jar = jar
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient: httpClient,
		ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{
			Host: u.Host, PathPrefixes: []string{"/resource/"},
		}}},
	})
	require.NoError(t, err)
	ref, err := client.ParseResourceRef(server.URL + "/resource/file.jpg")
	require.NoError(t, err)

	response, err := client.OpenResource(context.Background(), pixiv.OpenResourceRequest{
		Ref: ref, Range: "bytes=2-", IfNoneMatch: `"etag"`, IfModifiedSince: "Wed, 21 Oct 2015 07:28:00 GMT",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, response.StatusCode)
	assert.Equal(t, []string{`"first"`, `"second"`}, response.Header.Values("ETag"))
	assert.Equal(t, "bytes 2-12/13", response.Header.Get("Content-Range"))
	assert.Equal(t, "image/jpeg", response.Header.Get("Content-Type"))
	assert.Equal(t, "11", response.Header.Get("Content-Length"))
	assert.Empty(t, response.Header.Get("Set-Cookie"))
	assert.Empty(t, response.Header.Get("X-Internal"))
	assert.Equal(t, "bytes=2-", gotHeaders.Get("Range"))
	assert.Equal(t, `"etag"`, gotHeaders.Get("If-None-Match"))
	assert.Equal(t, "Wed, 21 Oct 2015 07:28:00 GMT", gotHeaders.Get("If-Modified-Since"))
	assert.Empty(t, gotHeaders.Get("Authorization"))
	assert.Empty(t, gotHeaders.Get("Cookie"))
	assert.Equal(t, "identity", gotHeaders.Get("Accept-Encoding"))

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "stream-body", string(body))
	for _, cookie := range jar.Cookies(u) {
		assert.NotEqual(t, "secret", cookie.Name, "public resource 响应不得更新调用方 Jar")
	}
}

func TestOpenResourceRevalidatesRefsAndRejectsInvalidHeadersBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	httpClient := &http.Client{Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not be called")
	})}
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: httpClient})
	require.NoError(t, err)

	refs := []pixiv.ResourceRef{
		{URL: "http://127.0.0.1/private"},
		{URL: "https://i.pximg.net/img-original/%2e%2e/private"},
		{URL: "https://i.pximg.net/img-original/%252e%252e/%25zz"},
	}
	encoded, err := json.Marshal(pixiv.ResourceRef{URL: "https://evil.example/img-original/a.jpg"})
	require.NoError(t, err)
	var decoded pixiv.ResourceRef
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	refs = append(refs, decoded)
	for _, ref := range refs {
		_, err := client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: ref})
		assert.ErrorIs(t, err, pixiv.ErrForbidden)
	}
	_, err = client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg?bad=%zz"}})
	assert.ErrorIs(t, err, pixiv.ErrInvalidArgument)
	for _, request := range []pixiv.OpenResourceRequest{
		{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"}, Range: "bytes=0-1\r\nX-Evil: yes"},
		{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"}, IfNoneMatch: "bad\x00value"},
	} {
		_, err := client.OpenResource(context.Background(), request)
		assert.ErrorIs(t, err, pixiv.ErrInvalidArgument)
	}
	assert.Zero(t, calls.Load())
}

func TestOpenResourceReturns304WithoutReadingAndCallerCloses(t *testing.T) {
	body := &resourceTrackingBody{Reader: strings.NewReader("must-not-read")}
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotModified, Header: http.Header{"Etag": {`"same"`}}, Body: body}, nil
	})}})
	require.NoError(t, err)
	response, err := client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"}})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotModified, response.StatusCode)
	assert.Zero(t, body.reads.Load())
	require.NoError(t, response.Body.Close())
	assert.True(t, body.closed.Load())
}

func TestOpenResourceRejectsRedirectOutsidePolicyBeforeSecondRequest(t *testing.T) {
	var firstHits, deniedHits atomic.Int32
	denied := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { deniedHits.Add(1) }))
	defer denied.Close()
	first := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstHits.Add(1)
		http.Redirect(w, &http.Request{}, denied.URL+"/private", http.StatusFound)
	}))
	defer first.Close()
	u, err := url.Parse(first.URL)
	require.NoError(t, err)
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:     first.Client(),
		ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: u.Host, PathPrefixes: []string{"/resource/"}}}},
	})
	require.NoError(t, err)
	_, err = client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: first.URL + "/resource/a.jpg"}})
	require.ErrorIs(t, err, pixiv.ErrForbidden)
	assert.EqualValues(t, 1, firstHits.Load())
	assert.Zero(t, deniedHits.Load())
}

func TestOpenResourceRevalidatesEveryRedirectHop(t *testing.T) {
	var allowedHits, deniedHits atomic.Int32
	denied := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { deniedHits.Add(1) }))
	defer denied.Close()
	allowed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowedHits.Add(1)
		http.Redirect(w, r, denied.URL+"/resource/final.jpg", http.StatusFound)
	}))
	defer allowed.Close()
	first := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, allowed.URL+"/resource/next.jpg", http.StatusFound)
	}))
	defer first.Close()
	firstURL, err := url.Parse(first.URL)
	require.NoError(t, err)
	allowedURL, err := url.Parse(allowed.URL)
	require.NoError(t, err)
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient: first.Client(),
		ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{
			{Host: firstURL.Host, PathPrefixes: []string{"/resource/"}},
			{Host: allowedURL.Host, PathPrefixes: []string{"/resource/"}},
		}},
	})
	require.NoError(t, err)
	_, err = client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: first.URL + "/resource/start.jpg"}})
	require.ErrorIs(t, err, pixiv.ErrForbidden)
	assert.EqualValues(t, 1, allowedHits.Load())
	assert.Zero(t, deniedHits.Load())
}

func TestOpenResourceAllowsRelativeRedirectWithinPolicy(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/allowed/start.jpg" {
			http.Redirect(w, r, "/allowed/final.jpg?size=original", http.StatusFound)
			return
		}
		assert.Equal(t, "/allowed/final.jpg", r.URL.Path)
		_, _ = io.WriteString(w, "image")
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:     server.Client(),
		ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: u.Host, PathPrefixes: []string{"/allowed"}}}},
	})
	require.NoError(t, err)
	response, err := client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: server.URL + "/allowed/start.jpg"}})
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, "image", string(body))
}

func TestOpenResourcePreservesCallerCheckRedirect(t *testing.T) {
	redirectErr := errors.New("caller redirect policy")
	var hits atomic.Int32
	var callbackCalled atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/resource/next.jpg", http.StatusFound)
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	httpClient := server.Client()
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		callbackCalled.Store(true)
		return redirectErr
	}
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:     httpClient,
		ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: u.Host, PathPrefixes: []string{"/resource/"}}}},
	})
	require.NoError(t, err)
	_, err = client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: server.URL + "/resource/start.jpg"}})
	require.ErrorIs(t, err, pixiv.ErrUpstreamUnavailable)
	assert.NotErrorIs(t, err, redirectErr, "调用方 redirect 错误不应进入公开 cause")
	assert.True(t, callbackCalled.Load())
	assert.EqualValues(t, 1, hits.Load())
}

func TestOpenResourceValidatesURLAfterCallerRedirectMutation(t *testing.T) {
	var privateHits atomic.Int32
	privateURL := "http://127.0.0.1/private?token=canary-query"
	httpClient := &http.Client{
		Transport: resourceRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Host == "127.0.0.1" {
				privateHits.Add(1)
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("private canary body"))}, nil
			}
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": {"https://i.pximg.net/img-original/allowed.jpg"}},
				Body:       io.NopCloser(strings.NewReader("redirect")),
			}, nil
		}),
		CheckRedirect: func(next *http.Request, _ []*http.Request) error {
			mutated, err := url.Parse(privateURL)
			require.NoError(t, err)
			next.URL = mutated
			return nil
		},
	}
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	response, err := client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/start.jpg"}})
	assert.Nil(t, response)
	require.ErrorIs(t, err, pixiv.ErrForbidden)
	var typed *pixiv.Error
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, pixiv.OperationOpenResource, typed.Operation)
	assert.Empty(t, typed.Backend)
	assert.Zero(t, privateHits.Load())
	assert.NotContains(t, err.Error(), "canary")
}

func TestOpenResourceClonesAllowedResponseHeaders(t *testing.T) {
	upstream := http.Header{
		"Content-Range": {"bytes 0-1/4", "bytes 2-3/4"},
		"Etag":          {`"first"`, `"second"`},
	}
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusPartialContent, Header: upstream, Body: io.NopCloser(strings.NewReader("data"))}, nil
	})}})
	require.NoError(t, err)
	response, err := client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"}})
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, []string{"bytes 0-1/4", "bytes 2-3/4"}, response.Header.Values("Content-Range"))
	response.Header["Content-Range"][0] = "mutated"
	response.Header["Etag"][0] = "mutated"
	assert.Equal(t, "bytes 0-1/4", upstream["Content-Range"][0])
	assert.Equal(t, `"first"`, upstream["Etag"][0])
}

func TestOpenResourceUseLastResponseRejectsAndClosesRedirect(t *testing.T) {
	body := &resourceTrackingBody{Reader: strings.NewReader("canary redirect body")}
	var hits atomic.Int32
	httpClient := &http.Client{
		Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
			hits.Add(1)
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": {"/img-original/final.jpg?token=canary-query"}},
				Body:       body,
			}, nil
		}),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	response, err := client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/start.jpg"}})
	assert.Nil(t, response)
	require.Error(t, err)
	var typed *pixiv.Error
	require.True(t, errors.As(err, &typed))
	assert.Equal(t, pixiv.OperationOpenResource, typed.Operation)
	assert.Equal(t, pixiv.BackendResource, typed.Backend)
	assert.Equal(t, http.StatusFound, typed.UpstreamStatus)
	assert.True(t, body.closed.Load())
	assert.Zero(t, body.reads.Load())
	assert.EqualValues(t, 1, hits.Load())
	assert.NotContains(t, err.Error(), "canary")
}

func TestOpenResourcePreservesDefaultRedirectLimit(t *testing.T) {
	var hits atomic.Int32
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
		hits.Add(1)
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": {"https://i.pximg.net/img-original/next.jpg"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})}})
	require.NoError(t, err)
	_, err = client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/start.jpg"}})
	require.ErrorIs(t, err, pixiv.ErrUpstreamUnavailable)
	assert.EqualValues(t, 10, hits.Load())
}

func TestOpenResourceMapsStatusTransportAndContextWithoutLeaks(t *testing.T) {
	for _, tc := range []struct {
		status    int
		want      error
		retryable bool
	}{
		{http.StatusBadRequest, pixiv.ErrInvalidArgument, false},
		{http.StatusForbidden, pixiv.ErrForbidden, false},
		{http.StatusTooManyRequests, pixiv.ErrRateLimited, true},
		{http.StatusInternalServerError, pixiv.ErrUpstreamError, true},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			body := &resourceTrackingBody{Reader: strings.NewReader("canary-response-body")}
			client, err := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: body, Header: make(http.Header)}, nil
			})}})
			require.NoError(t, err)
			response, err := client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg?token=canary-query"}})
			assert.Nil(t, response)
			require.ErrorIs(t, err, tc.want)
			var typed *pixiv.Error
			require.True(t, errors.As(err, &typed))
			assert.Equal(t, pixiv.OperationOpenResource, typed.Operation)
			assert.Equal(t, pixiv.BackendResource, typed.Backend)
			assert.Equal(t, tc.status, typed.UpstreamStatus)
			assert.Equal(t, tc.retryable, typed.Retryable)
			assert.True(t, body.closed.Load())
			assert.Zero(t, body.reads.Load())
			assert.NotContains(t, err.Error(), "canary")
		})
	}

	transportCanary := errors.New("transport canary-secret")
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportCanary
	})}})
	require.NoError(t, err)
	_, err = client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"}})
	assert.ErrorIs(t, err, pixiv.ErrUpstreamUnavailable)
	assert.NotErrorIs(t, err, transportCanary)
	assert.NotContains(t, err.Error(), "canary")

	contextClient, newErr := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})}})
	require.NoError(t, newErr)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = contextClient.OpenResource(canceled, pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"}})
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, err, pixiv.ErrUpstreamUnavailable)
	deadline, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer deadlineCancel()
	_, err = contextClient.OpenResource(deadline, pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"}})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorIs(t, err, pixiv.ErrUpstreamUnavailable)
}

func TestDownloadStreamsCompleteResourceAndAtomicallyReplacesTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "image.jpg")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))
	body := &resourceTrackingBody{Reader: strings.NewReader("new-complete-image")}
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		assert.Empty(t, request.Header.Get("Range"))
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}, nil
	})}})
	require.NoError(t, err)
	ref, err := client.ParseResourceRef("https://i.pximg.net/img-original/a.jpg")
	require.NoError(t, err)

	require.NoError(t, client.Download(context.Background(), ref, target))
	payload, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new-complete-image", string(payload))
	assert.True(t, body.closed.Load())
	assertNoDownloadTemps(t, dir)
}

func TestDownloadFailuresPreserveTargetAndCleanTemporaryFile(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   func(context.CancelFunc) io.ReadCloser
		want   error
	}{
		{
			name: "copy", status: http.StatusOK, want: pixiv.ErrUpstreamUnavailable,
			body: func(context.CancelFunc) io.ReadCloser {
				return &resourceTrackingBody{Reader: resourceErrorReader{err: errors.New("copy canary")}}
			},
		},
		{
			name: "body close", status: http.StatusOK, want: pixiv.ErrUpstreamUnavailable,
			body: func(context.CancelFunc) io.ReadCloser {
				return &resourceCloseErrorBody{Reader: strings.NewReader("complete")}
			},
		},
		{
			name: "partial status", status: http.StatusPartialContent, want: pixiv.ErrUpstreamError,
			body: func(context.CancelFunc) io.ReadCloser {
				return &resourceTrackingBody{Reader: strings.NewReader("partial")}
			},
		},
		{
			name: "not modified status", status: http.StatusNotModified, want: pixiv.ErrUpstreamError,
			body: func(context.CancelFunc) io.ReadCloser {
				return &resourceTrackingBody{Reader: strings.NewReader("not modified")}
			},
		},
		{
			name: "upstream status", status: http.StatusInternalServerError, want: pixiv.ErrUpstreamError,
			body: func(context.CancelFunc) io.ReadCloser {
				return &resourceTrackingBody{Reader: strings.NewReader("error canary body")}
			},
		},
		{
			name: "cancel during copy", status: http.StatusOK, want: context.Canceled,
			body: func(cancel context.CancelFunc) io.ReadCloser { return &resourceCancelBody{cancel: cancel} },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "image.jpg")
			require.NoError(t, os.WriteFile(target, []byte("old-target"), 0o600))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := tc.body(cancel)
			client, err := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: body}, nil
			})}})
			require.NoError(t, err)
			err = client.Download(ctx, pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"}, target)
			require.ErrorIs(t, err, tc.want)
			var typed *pixiv.Error
			require.True(t, errors.As(err, &typed))
			assert.Equal(t, pixiv.OperationDownload, typed.Operation)
			if tc.status != http.StatusOK {
				assert.Equal(t, tc.status, typed.UpstreamStatus)
			}
			assert.NotContains(t, err.Error(), "canary")
			if tracked, ok := body.(*resourceTrackingBody); ok {
				assert.True(t, tracked.closed.Load(), "失败路径必须关闭资源 body")
			}
			payload, readErr := os.ReadFile(target)
			require.NoError(t, readErr)
			assert.Equal(t, "old-target", string(payload))
			assertNoDownloadTemps(t, dir)
		})
	}
}

func TestDownloadInvalidDestinationDoesNotUseNetwork(t *testing.T) {
	var calls atomic.Int32
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not hit")
	})}})
	require.NoError(t, err)
	ref := pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"}
	for _, destination := range []string{"", filepath.Join(t.TempDir(), "missing", "image.jpg")} {
		err := client.Download(context.Background(), ref, destination)
		require.ErrorIs(t, err, pixiv.ErrInvalidArgument)
		var typed *pixiv.Error
		require.True(t, errors.As(err, &typed))
		assert.Equal(t, pixiv.OperationDownload, typed.Operation)
		assert.Empty(t, typed.Backend)
	}
	assert.Zero(t, calls.Load())
}

func TestDownloadReplacementFailurePreservesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "existing-directory")
	require.NoError(t, os.Mkdir(target, 0o700))
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("image"))}, nil
	})}})
	require.NoError(t, err)
	err = client.Download(context.Background(), pixiv.ResourceRef{URL: "https://i.pximg.net/img-original/a.jpg"}, target)
	require.ErrorIs(t, err, pixiv.ErrInvalidArgument)
	info, statErr := os.Stat(target)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	assertNoDownloadTemps(t, dir)
}

func assertNoDownloadTemps(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".pixiv-download-*"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

type resourceRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f resourceRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type resourceTrackingBody struct {
	io.Reader
	reads  atomic.Int32
	closed atomic.Bool
}

type resourceErrorReader struct{ err error }

func (r resourceErrorReader) Read([]byte) (int, error) { return 0, r.err }

type resourceCloseErrorBody struct{ io.Reader }

func (b *resourceCloseErrorBody) Close() error { return errors.New("close canary") }

type resourceCancelBody struct {
	cancel context.CancelFunc
	read   bool
}

func (b *resourceCancelBody) Read(buffer []byte) (int, error) {
	if b.read {
		return 0, context.Canceled
	}
	b.read = true
	b.cancel()
	buffer[0] = 'x'
	return 1, nil
}

func (b *resourceCancelBody) Close() error { return nil }

func (b *resourceTrackingBody) Read(buffer []byte) (int, error) {
	b.reads.Add(1)
	return b.Reader.Read(buffer)
}

func (b *resourceTrackingBody) Close() error {
	b.closed.Store(true)
	return nil
}

func TestParseResourceRefAcceptsKnownPixivPathsAndRoundTripsJSON(t *testing.T) {
	client, err := pixiv.NewClient(pixiv.Options{})
	require.NoError(t, err)

	urls := []string{
		"https://i.pximg.net/img-original/img/2024/01/02/03/04/05/1_p0.jpg?foo=bar&x=1",
		"https://i.pximg.net/img-master/img/2024/01/02/03/04/05/1_p0_master1200.jpg",
		"https://i.pximg.net/img-zip-ugoira/img/2024/01/02/03/04/05/1_ugoira600x600.zip",
		"https://i.pximg.net/c/250x250_80_a2/img-master/img/2024/01/02/03/04/05/1_p0_square1200.jpg",
	}
	for _, rawURL := range urls {
		t.Run(rawURL, func(t *testing.T) {
			ref, err := client.ParseResourceRef(rawURL)
			require.NoError(t, err)
			payload, err := json.Marshal(ref)
			require.NoError(t, err)
			var decoded pixiv.ResourceRef
			require.NoError(t, json.Unmarshal(payload, &decoded))
			assert.Equal(t, ref, decoded)
			assert.Equal(t, rawURL, decoded.URL, "合法 query 必须原样保留")
		})
	}
}

func TestParseResourceRefRejectsUnsafeURLsLocally(t *testing.T) {
	client, err := pixiv.NewClient(pixiv.Options{})
	require.NoError(t, err)

	tests := []struct {
		url  string
		want error
	}{
		{"http://i.pximg.net/img-original/a.jpg", pixiv.ErrForbidden},
		{"https://i.pximg.net.evil.test/img-original/a.jpg", pixiv.ErrForbidden},
		{"https://user@i.pximg.net/img-original/a.jpg", pixiv.ErrForbidden},
		{"https://i.pximg.net/img-original/a.jpg#fragment", pixiv.ErrForbidden},
		{"https://i.pximg.net/unknown/a.jpg", pixiv.ErrForbidden},
		{"https://i.pximg.net/img-original/%2e%2e/secret", pixiv.ErrForbidden},
		{"https://i.pximg.net/img-original/%252e%252e/secret", pixiv.ErrForbidden},
		{"https://i.pximg.net/img-original/%252e%252e/%25zz", pixiv.ErrForbidden},
		{"https://i.pximg.net/img-original/a.jpg?bad=%zz", pixiv.ErrInvalidArgument},
		{"https://127.0.0.1/img-original/a.jpg", pixiv.ErrForbidden},
		{"file:///img-original/a.jpg", pixiv.ErrInvalidArgument},
		{"data:image/jpeg;base64,AAAA", pixiv.ErrInvalidArgument},
		{"https://i.pximg.net", pixiv.ErrInvalidArgument},
		{"https://i.pximg.net:8443/img-original/a.jpg", pixiv.ErrForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			_, err := client.ParseResourceRef(tc.url)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.want)
			var typed *pixiv.Error
			require.True(t, errors.As(err, &typed))
			assert.Equal(t, pixiv.OperationParseResourceRef, typed.Operation)
			assert.Empty(t, typed.Backend)
			assert.NotContains(t, err.Error(), tc.url)
		})
	}
}

func TestResourceMirrorPolicyRequiresExactHostAndPathPrefix(t *testing.T) {
	client, err := pixiv.NewClient(pixiv.Options{ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{
		Host: "mirror.example:8443", PathPrefixes: []string{"/pixiv/images/", "/ugoira/"},
	}}}})
	require.NoError(t, err)

	_, err = client.ParseResourceRef("https://mirror.example:8443/pixiv/images/1.jpg")
	require.NoError(t, err)
	for _, rawURL := range []string{
		"https://mirror.example/pixiv/images/1.jpg",
		"https://mirror.example:8443/other/1.jpg",
		"https://mirror.example.evil:8443/pixiv/images/1.jpg",
	} {
		_, err := client.ParseResourceRef(rawURL)
		assert.ErrorIs(t, err, pixiv.ErrForbidden)
	}
}

func TestResourceMirrorPathPrefixHonorsSegmentBoundary(t *testing.T) {
	client, err := pixiv.NewClient(pixiv.Options{ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{
		Host: "mirror.example", PathPrefixes: []string{"/allowed"},
	}}}})
	require.NoError(t, err)
	for _, rawURL := range []string{"https://mirror.example/allowed", "https://mirror.example/allowed/image.jpg"} {
		_, err := client.ParseResourceRef(rawURL)
		require.NoError(t, err)
	}
	_, err = client.ParseResourceRef("https://mirror.example/allowed-evil/image.jpg")
	require.ErrorIs(t, err, pixiv.ErrForbidden)

	_, err = pixiv.NewClient(pixiv.Options{ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{
		Host: "mirror.example", PathPrefixes: []string{"/"},
	}}}})
	require.ErrorIs(t, err, pixiv.ErrInvalidArgument)
}

func TestOpenAndRedirectRejectMirrorPrefixBoundaryWithoutDeniedHit(t *testing.T) {
	var directCalls atomic.Int32
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient: &http.Client{Transport: resourceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
			directCalls.Add(1)
			return nil, errors.New("must not hit")
		})},
		ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: "mirror.example", PathPrefixes: []string{"/allowed"}}}},
	})
	require.NoError(t, err)
	_, err = client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: "https://mirror.example/allowed-evil/a.jpg"}})
	require.ErrorIs(t, err, pixiv.ErrForbidden)
	assert.Zero(t, directCalls.Load())

	var deniedHits atomic.Int32
	denied := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { deniedHits.Add(1) }))
	defer denied.Close()
	first := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, denied.URL+"/allowed-evil/a.jpg", http.StatusFound)
	}))
	defer first.Close()
	firstURL, err := url.Parse(first.URL)
	require.NoError(t, err)
	deniedURL, err := url.Parse(denied.URL)
	require.NoError(t, err)
	redirectClient, err := pixiv.NewClient(pixiv.Options{
		HTTPClient: first.Client(),
		ResourcePolicy: pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{
			{Host: firstURL.Host, PathPrefixes: []string{"/allowed"}},
			{Host: deniedURL.Host, PathPrefixes: []string{"/allowed"}},
		}},
	})
	require.NoError(t, err)
	_, err = redirectClient.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: pixiv.ResourceRef{URL: first.URL + "/allowed/start.jpg"}})
	require.ErrorIs(t, err, pixiv.ErrForbidden)
	assert.Zero(t, deniedHits.Load())
}

func TestNewClientRejectsInvalidResourceMirrorPolicy(t *testing.T) {
	policies := []pixiv.ResourcePolicy{
		{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: "https://mirror.example", PathPrefixes: []string{"/images/"}}}},
		{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: "mirror.example"}}},
		{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: "mirror.example", PathPrefixes: []string{"relative/"}}}},
		{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: "mirror.example", PathPrefixes: []string{"/a/../b/"}}}},
		{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: "mirror.example:bad", PathPrefixes: []string{"/images/"}}}},
		{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: "mirror.example", PathPrefixes: []string{"/images?scope=/"}}}},
		{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: "mirror.example", PathPrefixes: []string{"/"}}}},
	}
	for _, policy := range policies {
		_, err := pixiv.NewClient(pixiv.Options{ResourcePolicy: policy})
		require.ErrorIs(t, err, pixiv.ErrInvalidArgument)
		var typed *pixiv.Error
		require.True(t, errors.As(err, &typed))
		assert.Empty(t, typed.Backend)
	}
}
