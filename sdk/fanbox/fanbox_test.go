package fanbox_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func homeMetadataBody(userID int64, name string) string {
	return `<html><head><meta name="metadata" content='{"context":{"user":{"userId":` + strconv.FormatInt(userID, 10) + `,"name":"` + name + `"}}}'></head></html>`
}

func testClient(t *testing.T, rt roundTripFunc) *fanbox.Client {
	t.Helper()
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, fanbox.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return client
}

func TestSessionIdentity(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "www.fanbox.cc" || !strings.Contains(req.Header.Get("Cookie"), "FANBOXSESSID") {
			t.Errorf("request = %s cookie=%q", req.URL, req.Header.Get("Cookie"))
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
	})
	client := testClient(t, rt)
	if err := client.ValidateSession(context.Background()); err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	user, err := client.CurrentUser(context.Background(), fanbox.CurrentUserRequest{})
	if err != nil || user.UserID != 42 || user.DisplayName != "tester" {
		t.Fatalf("user = %+v err=%v", user, err)
	}

	expired := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: http.NoBody}, nil
	}))
	if err := expired.ValidateSession(context.Background()); sdk.ReasonOf(err) != sdk.CredentialsExpired {
		t.Fatalf("expected CredentialsExpired, got %v", err)
	}
}

func TestOpenWithOptions(t *testing.T) {
	const wantUA = "fanbox-test-agent/1"
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, fanbox.Options{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got := req.Header.Get("User-Agent"); got != wantUA {
				t.Errorf("User-Agent = %q, want %q", got, wantUA)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
		})},
		UserAgent: wantUA,
	})
	require.NoError(t, err)
	require.NoError(t, client.ValidateSession(context.Background()))

	called := false
	client, err = fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, fanbox.Options{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected transport call")
		})},
		UserAgent: "invalid\nagent",
	})
	require.Error(t, err)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	require.Nil(t, client)
	require.False(t, called)

	for _, test := range []struct {
		name    string
		options fanbox.Options
	}{
		{name: "service userinfo", options: fanbox.Options{FlareSolverr: &fanbox.FlareSolverrOptions{URL: "http://user:pass@solver.example"}}},
		{name: "service path prefix", options: fanbox.Options{FlareSolverr: &fanbox.FlareSolverrOptions{URL: "http://solver.example/prefix"}}},
		{name: "upstream userinfo", options: fanbox.Options{FlareSolverr: &fanbox.FlareSolverrOptions{URL: "http://solver.example", ProxyURL: "socks5://user:pass@proxy.example"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, test.options)
			require.Error(t, err)
			require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
			require.NotContains(t, err.Error(), "user")
			require.NotContains(t, err.Error(), "pass")
		})
	}
}

func TestCreatorsPagination(t *testing.T) {
	const nextURL = "https://api.fanbox.cc/plan.listSupporting?page=2"
	apiCalls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.fanbox.cc" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
		}
		if req.URL.Host != "api.fanbox.cc" || req.URL.Path != "/plan.listSupporting" {
			t.Errorf("request = %s", req.URL.String())
		}
		apiCalls++
		if apiCalls == 1 {
			return jsonResponse(`{"body":{"plans":[{"creatorId":"supported-1"}],"pageUrls":["` + nextURL + `"]}}`), nil
		}
		if req.URL.String() != nextURL {
			t.Errorf("continuation request = %s, want %s", req.URL.String(), nextURL)
		}
		return jsonResponse(`{"body":{"plans":[{"creatorId":"supported-2"}]}}`), nil
	})
	client := testClient(t, rt)
	page, err := client.Creators(context.Background(), fanbox.CreatorsRequest{Kind: fanbox.CreatorListSupporting})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "supported-1" || page.Next.IsZero() {
		t.Fatalf("page 1 = %+v err=%v", page, err)
	}
	page2, err := client.Creators(context.Background(), fanbox.CreatorsRequest{Kind: fanbox.CreatorListSupporting, Cursor: page.Next})
	if err != nil || len(page2.Items) != 1 || page2.Items[0].ID != "supported-2" || apiCalls != 2 {
		t.Fatalf("page 2 = %+v apiCalls=%d err=%v", page2, apiCalls, err)
	}
}

func TestPostMapsAssetsAndBody(t *testing.T) {
	body := `{"body":{"post":{"id":"p1","title":"hello","publishedDatetime":"2024-06-01T10:00:00Z","creatorId":"pixiv","feeRequired":0,"isRestricted":false,"isPinned":false,"body":{"text":"caption","images":[{"id":"img1","extension":"png","originalUrl":"https://i.pximg.net/img/1.png","thumbnailUrl":"https://i.pximg.net/img/1_th.png"}],"files":[]}}}}`
	client := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.fanbox.cc" || req.URL.Path != "/post.info" {
			t.Errorf("request = %s %s", req.Method, req.URL)
		}
		return jsonResponse(body), nil
	}))
	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p1"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if post.ID != "p1" || post.Title != "hello" || post.CreatorID != "pixiv" || post.Body == nil || len(post.Body.Assets) != 1 || post.Body.Assets[0].Resource.URL == "" {
		t.Fatalf("post = %+v", post)
	}
	if post.Body.Assets[0].Resource.RequestHeaders["Referer"] == "" {
		t.Fatal("image resource should carry a referer")
	}
}

func TestPostMapsFileMapWhenFilesListIsOmitted(t *testing.T) {
	body := `{"body":{"post":{"id":"p-file-map","title":"file map","publishedDatetime":"2024-06-01T10:00:00Z","creatorId":"pixiv","feeRequired":0,"isRestricted":false,"isPinned":false,"body":{"blocks":[{"type":"file","fileId":"file-1"}],"fileMap":{"file-1":{"id":"file-1","name":"clip.mp4","extension":"mp4","url":"https://downloads.fanbox.cc/file-1.mp4"}}}}}}`
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) { return jsonResponse(body), nil }))
	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p-file-map"})
	require.NoError(t, err)
	require.NotNil(t, post.Body)
	require.Len(t, post.Body.Assets, 1)
	require.Equal(t, fanbox.AssetKindFile, post.Body.Assets[0].Kind)
	require.Equal(t, "clip.mp4", post.Body.Assets[0].Name)
	require.Equal(t, "https://downloads.fanbox.cc/file-1.mp4", post.Body.Assets[0].Resource.URL)
	require.Len(t, post.Body.Blocks, 1)
	require.NotNil(t, post.Body.Blocks[0].File)
	require.Equal(t, "clip.mp4", post.Body.Blocks[0].File.Name)
}

func TestOpenResourceCredentialBoundary(t *testing.T) {
	for name, test := range map[string]struct {
		mediaHost, mediaURL string
		wantCookie          string
	}{
		"downloads host": {"downloads.fanbox.cc", "https://downloads.fanbox.cc/image-1.png", "FANBOXSESSID=session-value"},
		"pixiv media":    {"i.pximg.net", "https://i.pximg.net/image-1.png", ""},
	} {
		t.Run(name, func(t *testing.T) {
			var mediaCookie string
			client := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info":
					return jsonResponse(`{"body":{"post":{"id":"p-resource","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"` + test.mediaURL + `"}]}}}}`), nil
				case req.URL.Host == test.mediaHost:
					mediaCookie = req.Header.Get("Cookie")
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("bytes"))}, nil
				default:
					return nil, errors.New("unexpected FANBOX resource request")
				}
			}))
			post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p-resource"})
			if err != nil {
				t.Fatalf("Post: %v", err)
			}
			response, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: post.Body.Assets[0].Resource.Ref, Method: sdk.ResourceMethodGet})
			if err != nil {
				t.Fatalf("OpenResource: %v", err)
			}
			defer response.Body.Close()
			if mediaCookie != test.wantCookie {
				t.Fatalf("media Cookie = %q, want %q", mediaCookie, test.wantCookie)
			}
		})
	}
}

func TestOpenResourceForwardsMethodAndConditionalHeaders(t *testing.T) {
	var mediaRequest *http.Request
	client := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info":
			return jsonResponse(`{"body":{"post":{"id":"p-conditional","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"https://downloads.fanbox.cc/image-1.png"}]}}}}`), nil
		case req.URL.Host == "downloads.fanbox.cc":
			mediaRequest = req.Clone(req.Context())
			return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Length": {"2"}}, Body: io.NopCloser(strings.NewReader("ok"))}, nil
		default:
			return nil, errors.New("unexpected FANBOX resource request")
		}
	}))
	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p-conditional"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	response, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{
		Ref: post.Body.Assets[0].Resource.Ref, Method: sdk.ResourceMethodHead, IfNoneMatch: `"etag"`, Range: "bytes=0-1",
	})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer response.Body.Close()
	if mediaRequest == nil || mediaRequest.Method != http.MethodHead || mediaRequest.Header.Get("If-None-Match") != `"etag"` || mediaRequest.Header.Get("Range") != "bytes=0-1" {
		t.Fatalf("media request = %#v", mediaRequest)
	}
	if response.StatusCode != http.StatusPartialContent || response.ContentLength() != 2 {
		t.Fatalf("response = %d/%d", response.StatusCode, response.ContentLength())
	}
}

func TestIdentityScopedCursor(t *testing.T) {
	const nextURL = "https://api.fanbox.cc/post.listHome?maxPublishedDatetime=2024-01-01T00:00:00Z"
	mint := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.fanbox.cc" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "account-a")))}, nil
		}
		return jsonResponse(`{"body":{"posts":[{"id":"home-1","title":"a","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"pageUrls":["` + nextURL + `"]}}`), nil
	}))
	page, err := mint.Home(context.Background(), fanbox.HomeRequest{})
	require.NoError(t, err)
	require.False(t, page.Next.IsZero())

	consumeFeedCalled := false
	consumer := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.fanbox.cc" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(99, "account-b")))}, nil
		}
		consumeFeedCalled = true
		return nil, errors.New("account B feed must not be fetched with account A's cursor")
	}))
	if _, err := consumer.Home(context.Background(), fanbox.HomeRequest{Cursor: page.Next}); sdk.ReasonOf(err) != sdk.InvalidCursor || consumeFeedCalled {
		t.Fatalf("cross-account continuation = %v (fetched=%t)", err, consumeFeedCalled)
	}
}

func TestIdentityScopedCursorContinuesSameAccount(t *testing.T) {
	const nextURL = "https://api.fanbox.cc/post.listHome?maxPublishedDatetime=2024-01-01T00:00:00Z"
	calls := 0
	client := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.fanbox.cc" {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
		}
		calls++
		if calls == 1 {
			return jsonResponse(`{"body":{"posts":[{"id":"home-1","title":"a","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"pageUrls":["` + nextURL + `"]}}`), nil
		}
		if req.URL.String() != nextURL {
			t.Errorf("continuation request = %s, want %s", req.URL.String(), nextURL)
		}
		return jsonResponse(`{"body":{"posts":[{"id":"home-2","title":"b","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}]}}`), nil
	}))
	page, err := client.Home(context.Background(), fanbox.HomeRequest{})
	require.NoError(t, err)
	page2, err := client.Home(context.Background(), fanbox.HomeRequest{Cursor: page.Next})
	require.NoError(t, err)
	if len(page2.Items) != 1 || page2.Items[0].ID != "home-2" || calls != 2 {
		t.Fatalf("page 2 = %+v calls=%d", page2.Items, calls)
	}
}

func TestSessionCredentialsRedaction(t *testing.T) {
	credentials := fanbox.SessionCredentials{FANBOXSESSID: "session-canary"}
	for _, rendered := range []string{credentials.String(), credentials.GoString(), fmt.Sprintf("%v", credentials)} {
		if strings.Contains(rendered, "session-canary") {
			t.Fatalf("credentials leaked session value: %q", rendered)
		}
	}
}

func TestParallelClientsKeepSessionCookiesIsolated(t *testing.T) {
	type observation struct{ cookie string }
	makeClient := func(session, postID string, seen *observation) *fanbox.Client {
		client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: session}, fanbox.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seen.cookie = req.Header.Get("Cookie")
			return jsonResponse(`{"body":{"post":{"id":"` + postID + `","title":"p","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false}}}`), nil
		})}})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	firstObservation, secondObservation := &observation{}, &observation{}
	first := makeClient("first-session", "first-post", firstObservation)
	second := makeClient("second-session", "second-post", secondObservation)
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		if _, err := first.Post(context.Background(), fanbox.PostRequest{PostID: "first-post"}); err != nil {
			t.Errorf("first client error = %v", err)
		}
	}()
	go func() {
		defer group.Done()
		if _, err := second.Post(context.Background(), fanbox.PostRequest{PostID: "second-post"}); err != nil {
			t.Errorf("second client error = %v", err)
		}
	}()
	group.Wait()
	if firstObservation.cookie != "FANBOXSESSID=first-session" || secondObservation.cookie != "FANBOXSESSID=second-session" {
		t.Fatalf("cookies = %q / %q", firstObservation.cookie, secondObservation.cookie)
	}
}
