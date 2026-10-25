package pixiv_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// originalArtworkURL 是典型 img-original 路径，带签名查询；变体推导只改写
// path 并丢弃 query/fragment。
const originalArtworkURL = "https://i.pximg.net/img-original/img/2026/01/01/00/00/00/42_p0.png?signature=sentinel"

// TestArtworkVariantResource 覆盖质量变体映射：稳定身份、img-master 推导、
// 原图透传与非 artwork 拒绝。
func TestArtworkVariantResource(t *testing.T) {
	var requested string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "i.pximg.net" {
			requested = req.URL.String()
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/jpeg"}}, Body: http.NoBody}, nil
		}
		return jsonResponse(`{"illust":{"id":42,"title":"art","type":"illust","create_date":"2026-01-01T00:00:00Z","image_urls":{"original":"` + originalArtworkURL + `"},"user":{"id":7,"name":"artist"},"tags":[]}}`), nil
	})
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	artwork, err := client.Artwork(context.Background(), pixiv.ArtworkRequest{ArtworkID: 42})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	original := artwork.Cover.Resource
	for _, test := range []struct{ variant, suffix, crop string }{
		{variant: "regular", suffix: "_master1200", crop: "c/1200x1200/"},
		{variant: "small", suffix: "_master1200", crop: "c/540x540_70/"},
		{variant: "thumb", suffix: "_square1200", crop: "c/250x250_80_a2/"},
		{variant: "mini", suffix: "_square1200", crop: "c/48x48/"},
	} {
		t.Run(test.variant, func(t *testing.T) {
			ref, err := pixiv.ArtworkVariantResource(original, test.variant)
			if err != nil {
				t.Fatalf("ArtworkVariantResource: %v", err)
			}
			payload, err := sdk.ResourceRefPayload(ref)
			if err != nil || !strings.Contains(string(payload), `"v":"`+test.variant+`"`) {
				t.Fatalf("variant ref payload = %q err=%v", payload, err)
			}
			requested = ""
			if _, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: ref}); err != nil {
				t.Fatalf("OpenResource: %v", err)
			}
			if !strings.HasPrefix(requested, "https://i.pximg.net/"+test.crop+"img-master/img/") ||
				!strings.HasSuffix(requested, "42_p0"+test.suffix+".jpg") || strings.Contains(requested, "signature") {
				t.Fatalf("variant URL = %q", requested)
			}
		})
	}

	for _, variant := range []string{"", "original"} {
		if got, err := pixiv.ArtworkVariantResource(original, variant); err != nil || got != original.Ref {
			t.Fatalf("variant %q = %v err=%v, want passthrough", variant, got, err)
		}
	}
	novelRef, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"novel_cover","id":42}`))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	if _, err := pixiv.ArtworkVariantResource(sdk.Resource{Ref: novelRef}, "regular"); err == nil {
		t.Fatal("ArtworkVariantResource should reject non-artwork kind")
	}

	foreign, err := sdk.NewResourceRef("other-product", []byte(`{"k":"artwork","id":42}`))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	if _, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: foreign}); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("foreign product = %v, want InvalidArgument", err)
	}
}

func TestSaveResourceURLWritesAtomically(t *testing.T) {
	const body = "DATA"
	var received *http.Request
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Clone(r.Context())
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", "4")
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{Name: "secret", Value: "must-not-send"}})
	httpClient := server.Client()
	httpClient.Jar = jar
	client, err := pixiv.NewWith("token", pixiv.Options{
		HTTPClient:     httpClient,
		ResourcePolicy: pixiv.ResourcePolicy{AllowedHosts: []string{serverURL.Hostname()}},
	})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "nested", "asset.png")
	var progress []sdk.SaveProgress
	saved, err := client.SaveResourceURL(context.Background(), server.URL+"/img/asset.png?signature=secret", sdk.SaveOptions{
		Path:     destination,
		Progress: func(value sdk.SaveProgress) { progress = append(progress, value) },
	})
	if err != nil {
		t.Fatalf("SaveResourceURL: %v", err)
	}
	if saved.Path != destination || saved.Size != int64(len(body)) || saved.ContentType != "image/png" {
		t.Fatalf("saved = %+v", saved)
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != body {
		t.Fatalf("content = %q err=%v", content, err)
	}
	if received == nil || received.Method != http.MethodGet || received.Header.Get("Referer") != "https://app-api.pixiv.net/" || received.Header.Get("Cookie") != "" {
		t.Fatalf("received request = %#v", received)
	}
	if len(progress) == 0 || progress[len(progress)-1].Total != int64(len(body)) || progress[len(progress)-1].Done != int64(len(body)) {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestSaveResourceURLRejectsInvalidInputsWithoutDestination(t *testing.T) {
	calls := 0
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("invalid resource reached upstream")
	})}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	for name, test := range map[string]struct {
		raw      string
		reason   sdk.Reason
		redirect string
		status   int
	}{
		"http scheme":          {raw: "http://i.pximg.net/img/asset.png", reason: sdk.ResourceForbidden},
		"userinfo":             {raw: "https://user:pass@i.pximg.net/img/asset.png", reason: sdk.ResourceForbidden},
		"missing path":         {raw: "https://i.pximg.net/", reason: sdk.ResourceForbidden},
		"foreign signed host":  {raw: "https://example.com/img/asset.png?signature=secret", reason: sdk.ResourceForbidden},
		"disallowed redirect":  {raw: "https://i.pximg.net/img/asset.png", redirect: "https://example.com/asset.png"},
		"non-success response": {raw: "https://i.pximg.net/img/asset.png", reason: sdk.UpstreamError, status: http.StatusNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			target := test.raw
			if test.redirect != "" || test.status != 0 {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if test.redirect != "" {
						http.Redirect(w, r, test.redirect, http.StatusFound)
						return
					}
					w.WriteHeader(test.status)
					_, _ = io.WriteString(w, "private upstream body")
				}))
				defer server.Close()
				serverURL, err := url.Parse(server.URL)
				if err != nil {
					t.Fatalf("url.Parse: %v", err)
				}
				target = server.URL + "/img/asset.png"
				client, err = pixiv.NewWith("token", pixiv.Options{
					HTTPClient:     server.Client(),
					ResourcePolicy: pixiv.ResourcePolicy{AllowedHosts: []string{serverURL.Hostname()}},
				})
				if err != nil {
					t.Fatalf("NewWith: %v", err)
				}
			}
			destination := filepath.Join(t.TempDir(), "asset.bin")
			_, err := client.SaveResourceURL(context.Background(), target, sdk.SaveOptions{Path: destination})
			if err == nil {
				t.Fatal("SaveResourceURL unexpectedly succeeded")
			}
			if test.reason != "" && sdk.ReasonOf(err) != test.reason {
				t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), test.reason, err)
			}
			for _, leaked := range []string{"signature=secret", "example.com", "blocked"} {
				if strings.Contains(err.Error(), leaked) {
					t.Fatalf("error leaked %q: %v", leaked, err)
				}
			}
			if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
				t.Fatalf("destination should not exist, stat error = %v", statErr)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid resource reached upstream %d time(s)", calls)
	}
}

func TestSaveResourceDoesNotPublishPartialBody(t *testing.T) {
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(&failingBody{})}, nil
	})}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "asset.png")
	_, err = client.SaveResourceURL(context.Background(), "https://i.pximg.net/img/asset.png", sdk.SaveOptions{Path: destination})
	if sdk.ReasonOf(err) != sdk.LocalStateError {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.LocalStateError, err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("destination should not exist, stat error = %v", statErr)
	}
}

func TestSaveResourcePreservesWriteContextCause(t *testing.T) {
	for _, test := range []struct {
		name string
		url  string
		ref  bool
	}{
		{name: "url", url: "https://i.pximg.net/img/asset.png"},
		{name: "ref", ref: true},
	} {
		for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
			t.Run(test.name+"/"+cause.Error(), func(t *testing.T) {
				rawURL := "https://i.pximg.net/img/asset.png"
				client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if req.URL.Host == "app-api.pixiv.net" {
						return jsonResponse(`{"illust":{"id":42,"title":"art","type":"illust","create_date":"2026-01-01T00:00:00Z","image_urls":{"original":"` + rawURL + `"},"user":{"id":7,"name":"artist"},"tags":[]}}`), nil
					}
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(&contextErrorBody{cause: cause})}, nil
				})}})
				if err != nil {
					t.Fatalf("NewWith: %v", err)
				}
				destination := filepath.Join(t.TempDir(), "asset.png")
				if !test.ref {
					_, err = client.SaveResourceURL(context.Background(), test.url, sdk.SaveOptions{Path: destination})
				} else {
					artwork, artworkErr := client.Artwork(context.Background(), pixiv.ArtworkRequest{ArtworkID: 42})
					if artworkErr != nil {
						t.Fatalf("Artwork: %v", artworkErr)
					}
					_, err = client.SaveResource(context.Background(), artwork.Cover.Resource.Ref, sdk.SaveOptions{Path: destination})
				}
				if err == nil || !errors.Is(err, cause) || sdk.ReasonOf(err) != sdk.LocalStateError {
					t.Fatalf("err = %v, want %v with LocalStateError", err, cause)
				}
				if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
					t.Fatalf("destination should not exist, stat error = %v", statErr)
				}
			})
		}
	}
}

func TestNovelImageResourceRejectsEmptyOrMalformedContent(t *testing.T) {
	for name, test := range map[string]struct {
		body string
		url  string
	}{
		"empty content":  {body: " \n\t"},
		"no image block": {body: `<html><body><p class="noveltext">text</p></body></html>`},
		"image block":    {body: `<html><body><figure class="image"><img src="https://i.pximg.net/novel/image.png"></figure></body></html>`, url: "https://i.pximg.net/novel/image.png"},
	} {
		t.Run(name, func(t *testing.T) {
			var requests []string
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests = append(requests, req.URL.Path)
				if req.URL.Host == "i.pximg.net" {
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: http.NoBody}, nil
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})
			client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			ref, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"novel_image","id":42,"p":0}`))
			if err != nil {
				t.Fatalf("NewResourceRef: %v", err)
			}
			response, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: ref})
			if test.url == "" {
				if err == nil || sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
					t.Fatalf("error = %v, want malformed response", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenResource: %v", err)
			}
			_ = response.Body.Close()
			if len(requests) == 0 || requests[len(requests)-1] != "/novel/image.png" {
				t.Fatalf("requests = %v, want the parsed novel image", requests)
			}
		})
	}
}

type contextErrorBody struct {
	written bool
	cause   error
}

func (b *contextErrorBody) Read(p []byte) (int, error) {
	if b.written {
		return 0, b.cause
	}
	b.written = true
	copy(p, "partial")
	return len("partial"), nil
}

func (b *contextErrorBody) Close() error { return nil }

type failingBody struct{ written bool }

func (b *failingBody) Read(p []byte) (int, error) {
	if b.written {
		return 0, errors.New("resource body read failed")
	}
	b.written = true
	copy(p, "partial")
	return len("partial"), nil
}

func (b *failingBody) Close() error { return nil }

// TestOpenResourceConditionalRequest 覆盖 OpenResource 的条件请求头转发、
// 请求不带 cookie，以及响应只暴露 allowlist 头。
func TestOpenResourceConditionalRequest(t *testing.T) {
	var received *http.Request
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received = req.Clone(req.Context())
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Range", "bytes 0-3/4")
		w.Header().Set("ETag", `"etag"`)
		w.Header().Set("Set-Cookie", "sentinel=must-not-leak")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "DATA")
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	httpClient := server.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{{Name: "sentinel", Value: "must-not-send"}})
	httpClient.Jar = jar
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "app-api.pixiv.net" {
			return jsonResponse(`{"illust":{"id":1,"title":"art","type":"illust","create_date":"2026-01-01T00:00:00Z","image_urls":{"original":"` + server.URL + `/asset"},"user":{"id":7,"name":"artist"},"tags":[]}}`), nil
		}
		return httpClient.Transport.RoundTrip(req)
	})
	client, err := pixiv.NewWith("token", pixiv.Options{
		HTTPClient:     &http.Client{Transport: rt, Jar: jar},
		ResourcePolicy: pixiv.ResourcePolicy{AllowedHosts: []string{serverURL.Hostname()}},
	})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	artwork, err := client.Artwork(context.Background(), pixiv.ArtworkRequest{ArtworkID: 1})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	response, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{
		Ref:   artwork.Cover.Resource.Ref,
		Range: "bytes=0-3", IfNoneMatch: `"old"`,
		IfModifiedSince: "Wed, 01 Jan 2025 00:00:00 GMT", IfRange: `"old"`,
	})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "DATA" {
		t.Fatalf("body = %q err=%v", body, err)
	}
	if received == nil || received.Header.Get("Range") != "bytes=0-3" || received.Header.Get("If-None-Match") != `"old"` ||
		received.Header.Get("If-Modified-Since") == "" || received.Header.Get("If-Range") != `"old"` {
		t.Fatalf("forwarded headers = %#v", received)
	}
	if received.Header.Get("Cookie") != "" {
		t.Fatalf("resource request carried a cookie: %q", received.Header.Get("Cookie"))
	}
	if response.Header().Get("Set-Cookie") != "" || response.ContentRange() != "bytes 0-3/4" || response.ETag() != `"etag"` {
		t.Fatalf("response headers = %#v", response.Header())
	}
}
