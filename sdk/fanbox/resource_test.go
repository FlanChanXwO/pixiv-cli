package fanbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

// TestFanboxResourceIdentityIsStable 验证资源 ref 只编码稳定身份，
// 不嵌入已解析的签名 URL；locator 轮换不改变 ref。
func TestFanboxResourceIdentityIsStable(t *testing.T) {
	signedURL := "https://downloads.fanbox.cc/image-1.png?signature=expires-soon"
	body := `{"body":{"post":{"id":"p-identity","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"` + signedURL + `"}]}}}}`
	client := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info" {
			return jsonResponse(body), nil
		}
		return nil, errors.New("unexpected request: " + req.URL.String())
	}))
	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p-identity"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	ref := post.Body.Assets[0].Resource.Ref
	payload, err := sdk.ResourceRefPayload(ref)
	if err != nil {
		t.Fatalf("ResourceRefPayload: %v", err)
	}
	for _, forbidden := range []string{"signature", "expires-soon", "downloads.fanbox.cc", "https://"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("resource ref payload leaked locator %q: %s", forbidden, payload)
		}
	}
	var decoded struct {
		Kind  string `json:"k"`
		Post  string `json:"p"`
		Asset string `json:"a"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Kind != "post_image" || decoded.Post != "p-identity" || decoded.Asset != "image-1" {
		t.Fatalf("resource ref payload = %+v", decoded)
	}
}

// TestFanboxResourceCredentialFlag 覆盖 downloads host 与 pixiv CDN 的凭据标记。
func TestFanboxResourceCredentialFlag(t *testing.T) {
	for name, test := range map[string]struct {
		url  string
		want bool
	}{
		"downloads host": {"https://downloads.fanbox.cc/img-dl.png", true},
		"pximg host":     {"https://i.pximg.net/img-px.png", false},
	} {
		t.Run(name, func(t *testing.T) {
			client := testClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info" {
					return jsonResponse(`{"body":{"post":{"id":"p","title":"x","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"img","originalUrl":"` + test.url + `"}]}}}}`), nil
				}
				return nil, errors.New("unexpected request: " + req.URL.String())
			}))
			post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p"})
			if err != nil {
				t.Fatalf("Post: %v", err)
			}
			if got := post.Body.Assets[0].Resource.RequiresCredentials; got != test.want {
				t.Fatalf("RequiresCredentials = %v, want %v", got, test.want)
			}
		})
	}
}

// TestFanboxOpenResourceReResolvesFreshLocator 验证无缓存 locator 的 ref 会
// 重新解析最新 URL，而不是重开过时内嵌 URL。
func TestFanboxOpenResourceReResolvesFreshLocator(t *testing.T) {
	originalURL := "https://downloads.fanbox.cc/image-1.png"
	postBody := `{"body":{"post":{"id":"p-reresolve","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"` + originalURL + `"}]}}}}`
	var mediaSeen string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info":
			return jsonResponse(postBody), nil
		case req.URL.Host == "downloads.fanbox.cc":
			mediaSeen = req.URL.String()
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("bytes"))}, nil
		default:
			return nil, errors.New("unexpected request: " + req.URL.String())
		}
	})
	producer := testClient(t, rt)
	post, err := producer.Post(context.Background(), fanbox.PostRequest{PostID: "p-reresolve"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	consumer := testClient(t, rt)
	response, err := consumer.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: post.Body.Assets[0].Resource.Ref, Method: sdk.ResourceMethodGet})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer response.Body.Close()
	if mediaSeen != originalURL {
		t.Fatalf("re-resolved media URL = %q, want %q", mediaSeen, originalURL)
	}
}

func TestFanboxOpenResourceRejectsForeignProduct(t *testing.T) {
	pixivRef, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42}`))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no network expected")
	}))
	if _, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: pixivRef}); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected InvalidArgument for foreign product ref, got %v", err)
	}
}

func TestFanboxResourceKindsSurviveReopen(t *testing.T) {
	for _, test := range []struct{ name, want string }{{"报告", "报告.pdf"}, {"report.PDF", "report.PDF"}, {"folder/report", "folder/report.pdf"}, {"", ""}} {
		t.Run(test.name, func(t *testing.T) {
			postBody := `{"body":{"post":{"id":"p-shared","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"blocks":[{"type":"image","imageId":"shared"},{"type":"file","fileId":"shared"}],"imageMap":{"shared":{"id":"shared","originalUrl":"https://i.pximg.net/image.png"}},"fileMap":{"shared":{"id":"shared","name":"报告","extension":"pdf","url":"https://downloads.fanbox.cc/file.pdf"}}}}}}`
			encodedName, err := json.Marshal(test.name)
			if err != nil {
				t.Fatal(err)
			}
			postBody = strings.Replace(postBody, `"name":"报告"`, `"name":`+string(encodedName), 1)
			posts := 0
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "api.fanbox.cc" {
					posts++
					return jsonResponse(postBody), nil
				}
				body := "image"
				if req.URL.Host == "downloads.fanbox.cc" {
					body = "file"
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/octet-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			first := testClient(t, rt)
			post, err := first.Post(t.Context(), fanbox.PostRequest{PostID: "p-shared"})
			if err != nil {
				t.Fatal(err)
			}
			if len(post.Body.Assets) != 2 {
				t.Fatalf("assets=%+v", post.Body.Assets)
			}
			for _, asset := range post.Body.Assets {
				want := "image"
				if asset.Kind == fanbox.AssetKindFile {
					want = "file"
				}
				for _, client := range []*fanbox.Client{first, testClient(t, rt)} {
					for range 2 {
						resp, err := client.OpenResource(t.Context(), sdk.OpenResourceRequest{Ref: asset.Resource.Ref})
						if err != nil {
							t.Fatal(err)
						}
						wantName := ""
						if asset.Kind == fanbox.AssetKindFile {
							wantName = test.want
						}
						if resp.Filename != wantName {
							t.Fatalf("filename=%q want %q", resp.Filename, wantName)
						}
						data, err := io.ReadAll(resp.Body)
						closeErr := resp.Body.Close()
						if err != nil || closeErr != nil {
							t.Fatalf("read/close: %v %v", err, closeErr)
						}
						if string(data) != want {
							t.Fatalf("resource kind=%s returned %q, want %q", asset.Kind, data, want)
						}
					}
				}
			}
			if posts != 3 {
				t.Fatalf("metadata reads=%d want 3 (one warm + one per reopened ref)", posts)
			}
		})
	}
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
