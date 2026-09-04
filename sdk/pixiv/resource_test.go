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

// originalArtworkURL 是一个典型的 img-original 路径，带签名查询参数；用于
// 验证质量变体推导只改写 path 并丢弃 query/fragment。
const originalArtworkURL = "https://i.pximg.net/img-original/img/2026/01/01/00/00/00/42_p0.png?signature=sentinel"

var errUnexpectedVariantHost = errors.New("unexpected variant host")

// variantCases 是公开 DownloadQuality 与 SDK variant 的映射，以及每个质量
// 推导出的 img-master 路径后缀与裁剪段。DownloadQuality 常量在 downloader
// 包里定义；这里用字面量避免把 downloader 依赖引入 sdk/pixiv 测试。
var variantCases = []struct {
	quality string
	variant string
	suffix  string
	crop    string
}{
	{"regular", "regular", "_master1200", "c/1200x1200/"},
	{"small", "small", "_master1200", "c/540x540_70/"},
	{"thumb", "thumb", "_square1200", "c/250x250_80_a2/"},
	{"mini", "mini", "_square1200", "c/48x48/"},
}

func TestArtworkVariantResourceDerivesImgMasterURL(t *testing.T) {
	// detail 返回带 original 路径的作品；OpenResource 再对变体 ref revalidate
	// 时会把 detail 端点重新拉一次，最终落到 i.pximg.net 的变体 URL。
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "app-api.pixiv.net":
			body := `{"illust":{"id":42,"title":"art","type":"illust","create_date":"2026-01-01T00:00:00Z","image_urls":{"original":"` + originalArtworkURL + `"},"user":{"id":7,"name":"artist"},"tags":[]}}`
			return jsonResponse(body), nil
		case "i.pximg.net":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"image/jpeg"}},
				Body:       io.NopCloser(strings.NewReader("DATA")),
			}, nil
		default:
			return nil, &url.Error{Op: "Get", URL: req.URL.String(), Err: errUnexpectedVariantHost}
		}
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
	for _, tc := range variantCases {
		t.Run(tc.quality, func(t *testing.T) {
			variantRef, err := pixiv.ArtworkVariantResource(original, tc.variant)
			if err != nil {
				t.Fatalf("ArtworkVariantResource: %v", err)
			}
			// 变体 ref 必须保留稳定身份并只替换 variant 字段。
			payload, err := sdk.ResourceRefPayload(variantRef)
			if err != nil {
				t.Fatalf("ResourceRefPayload: %v", err)
			}
			if !strings.Contains(string(payload), `"v":"`+tc.variant+`"`) {
				t.Fatalf("variant ref payload = %q, want variant %q", payload, tc.variant)
			}
			// 推导出的 URL 走 img-master，丢弃签名 query。
			gotURL, err := resolveVariantURL(t, client, variantRef)
			if err != nil {
				t.Fatalf("resolveVariantURL: %v", err)
			}
			if !strings.HasPrefix(gotURL, "https://i.pximg.net/"+tc.crop+"img-master/img/") {
				t.Fatalf("variant URL = %q, want prefix %q", gotURL, "https://i.pximg.net/"+tc.crop+"img-master/img/")
			}
			if !strings.HasSuffix(gotURL, "42_p0"+tc.suffix+".jpg") {
				t.Fatalf("variant URL = %q, want suffix %q", gotURL, "42_p0"+tc.suffix+".jpg")
			}
			if strings.Contains(gotURL, "signature") {
				t.Fatalf("variant URL must not carry the original signature query: %q", gotURL)
			}
		})
	}
}

func TestArtworkVariantResourceRejectsNonArtworkKind(t *testing.T) {
	novelRef, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"novel_cover","id":42}`))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	original := sdk.Resource{Ref: novelRef}
	if _, err := pixiv.ArtworkVariantResource(original, "regular"); err == nil {
		t.Fatal("ArtworkVariantResource should reject non-artwork kind")
	}
}

func TestArtworkVariantResourcePassesThroughOriginal(t *testing.T) {
	ref, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42,"p":0}`))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	original := sdk.Resource{Ref: ref}
	for _, variant := range []string{"", "original"} {
		got, err := pixiv.ArtworkVariantResource(original, variant)
		if err != nil {
			t.Fatalf("variant %q: %v", variant, err)
		}
		if got != ref {
			t.Fatalf("variant %q returned %v, want passthrough %v", variant, got, ref)
		}
	}
}

// resolveVariantURL 打开变体资源并返回它最终被请求到的 locator。因为 detail
// 只提供 original，变体 ref 必须在 revalidate 路径上被重新解析为 img-master URL。
func resolveVariantURL(t *testing.T, client *pixiv.Client, ref sdk.ResourceRef) (string, error) {
	t.Helper()
	var requested string
	capturing := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "i.pximg.net" {
			requested = req.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"image/jpeg"}},
				Body:       io.NopCloser(strings.NewReader("DATA")),
			}, nil
		}
		body := `{"illust":{"id":42,"title":"art","type":"illust","create_date":"2026-01-01T00:00:00Z","image_urls":{"original":"` + originalArtworkURL + `"},"user":{"id":7,"name":"artist"},"tags":[]}}`
		return jsonResponse(body), nil
	})
	captureClient, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: capturing}})
	if err != nil {
		return "", err
	}
	if _, err := captureClient.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: ref}); err != nil {
		return "", err
	}
	return requested, nil
}

func TestSaveResourceURLWritesAtomicFileAndReturnsMetadata(t *testing.T) {
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
		HTTPClient: httpClient,
		ResourcePolicy: pixiv.ResourcePolicy{
			AllowedHosts: []string{serverURL.Hostname()},
		},
	})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "nested", "asset.png")
	var progress []sdk.SaveProgress
	saved, err := client.SaveResourceURL(context.Background(), server.URL+"/img/asset.png?signature=secret", sdk.SaveOptions{
		Path: destination,
		Progress: func(value sdk.SaveProgress) {
			progress = append(progress, value)
		},
	})
	if err != nil {
		t.Fatalf("SaveResourceURL: %v", err)
	}
	if saved.Path != destination || saved.Size != int64(len(body)) || saved.ContentType != "image/png" {
		t.Fatalf("saved = %+v", saved)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != body {
		t.Fatalf("content = %q, want %q", content, body)
	}
	if received == nil || received.Method != http.MethodGet {
		t.Fatalf("received request = %#v", received)
	}
	if got := received.Header.Get("Referer"); got != "https://app-api.pixiv.net/" {
		t.Fatalf("Referer = %q", got)
	}
	if got := received.Header.Get("Cookie"); got != "" {
		t.Fatalf("resource request carried a cookie: %q", got)
	}
	if len(progress) == 0 || progress[len(progress)-1].Done != int64(len(body)) || progress[len(progress)-1].Total != int64(len(body)) {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestSaveResourceURLRejectsInvalidURLsBeforeNetwork(t *testing.T) {
	calls := 0
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("invalid resource reached upstream")
	})}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	tests := []struct {
		name string
		raw  string
	}{
		{name: "http scheme", raw: "http://i.pximg.net/img/asset.png"},
		{name: "userinfo", raw: "https://user:pass@i.pximg.net/img/asset.png"},
		{name: "missing path", raw: "https://i.pximg.net/"},
		{name: "foreign signed host", raw: "https://example.com/img/asset.png?signature=secret"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "asset.bin")
			_, err := client.SaveResourceURL(context.Background(), tc.raw, sdk.SaveOptions{Path: destination})
			if sdk.ReasonOf(err) != sdk.ResourceForbidden {
				t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.ResourceForbidden, err)
			}
			if strings.Contains(err.Error(), "signature=secret") {
				t.Fatalf("error leaked signed query: %v", err)
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

func TestSaveResourceURLRejectsDisallowedRedirectWithoutDestination(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/blocked.png?signature=secret", http.StatusFound)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	client, err := pixiv.NewWith("token", pixiv.Options{
		HTTPClient: server.Client(),
		ResourcePolicy: pixiv.ResourcePolicy{
			AllowedHosts: []string{serverURL.Hostname()},
		},
	})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "asset.bin")
	_, err = client.SaveResourceURL(context.Background(), server.URL+"/redirect.png?signature=secret", sdk.SaveOptions{Path: destination})
	if err == nil {
		t.Fatal("SaveResourceURL should reject a disallowed redirect")
	}
	if strings.Contains(err.Error(), "signature=secret") || strings.Contains(err.Error(), "example.com") {
		t.Fatalf("redirect error leaked upstream URL: %v", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("destination should not exist, stat error = %v", statErr)
	}
}

func TestSaveResourceURLRejectsNonSuccessWithoutDestination(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "private upstream body", http.StatusNotFound)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	client, err := pixiv.NewWith("token", pixiv.Options{
		HTTPClient: server.Client(),
		ResourcePolicy: pixiv.ResourcePolicy{
			AllowedHosts: []string{serverURL.Hostname()},
		},
	})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "asset.bin")
	_, err = client.SaveResourceURL(context.Background(), server.URL+"/missing.png", sdk.SaveOptions{Path: destination})
	if sdk.ReasonOf(err) != sdk.UpstreamError {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.UpstreamError, err)
	}
	if strings.Contains(err.Error(), "private upstream body") {
		t.Fatalf("status error leaked response body: %v", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatalf("destination should not exist, stat error = %v", statErr)
	}
}

func TestSaveResourceURLDoesNotPublishPartialBody(t *testing.T) {
	client, err := pixiv.NewWith("token", pixiv.Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"image/png"}},
			Body:       io.NopCloser(&failingBody{}),
		}, nil
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

type failingBody struct {
	written bool
}

func (b *failingBody) Read(p []byte) (int, error) {
	if b.written {
		return 0, errors.New("resource body read failed")
	}
	b.written = true
	copy(p, "partial")
	return len("partial"), nil
}

func (b *failingBody) Close() error { return nil }
