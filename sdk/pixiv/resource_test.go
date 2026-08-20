package pixiv_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
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
