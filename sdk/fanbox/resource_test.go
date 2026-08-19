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

// TestFanboxResourceRefEncodesIdentityNotLocator 验证 finding #5：FANBOX 资源 ref
// 的 payload 只编码稳定身份（kind + creator/post + attachment），绝不嵌入已解析的 URL
// 或其签名查询参数。locator 轮换不得改变 cache key。
func TestFanboxResourceRefEncodesIdentityNotLocator(t *testing.T) {
	signedURL := "https://downloads.fanbox.cc/image-1.png?signature=expires-soon"
	body := `{"body":{"post":{"id":"p-identity","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"` + signedURL + `"}]}}}}`
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info" {
			return jsonResponse(body), nil
		}
		return nil, errors.New("unexpected request: " + req.URL.String())
	})
	client := testClient(t, rt)
	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p-identity"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if len(post.Body.Assets) != 1 {
		t.Fatalf("post assets = %+v", post.Body.Assets)
	}
	ref := post.Body.Assets[0].Resource.Ref
	if ref.IsZero() {
		t.Fatal("resource ref is zero")
	}
	payload, err := sdk.ResourceRefPayload(ref)
	if err != nil {
		t.Fatalf("ResourceRefPayload: %v", err)
	}
	encoded := string(payload)
	for _, forbidden := range []string{"signature", "expires-soon", "downloads.fanbox.cc", "https://"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("resource ref payload leaked locator %q: %s", forbidden, encoded)
		}
	}
	// payload 必须携带稳定身份字段。
	var rp struct {
		Kind  string `json:"k"`
		Post  string `json:"p"`
		Asset string `json:"a"`
	}
	if err := json.Unmarshal(payload, &rp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if rp.Kind != "post_image" || rp.Post != "p-identity" || rp.Asset != "image-1" {
		t.Fatalf("resource ref payload = %+v, want post_image/p-identity/image-1", rp)
	}
}

// TestFanboxResourceMarksDownloadsHostCredentialed 验证 finding #19：落在
// downloads.fanbox.cc 的附件标记 RequiresCredentials，使调用方走 OpenResource
// 而非把 URL 当公开链接；Pixiv CDN 不标记。
func TestFanboxResourceMarksDownloadsHostCredentialed(t *testing.T) {
	cases := []struct {
		name     string
		postBody string
		host     string
		want     bool
	}{
		{
			name:     "downloads host",
			postBody: `{"body":{"post":{"id":"p-dl","title":"dl","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"img-dl","originalUrl":"https://downloads.fanbox.cc/img-dl.png"}]}}}}`,
			want:     true,
		},
		{
			name:     "pximg host",
			postBody: `{"body":{"post":{"id":"p-px","title":"px","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"img-px","originalUrl":"https://i.pximg.net/img-px.png"}]}}}}`,
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info" {
					return jsonResponse(tc.postBody), nil
				}
				return nil, errors.New("unexpected request: " + req.URL.String())
			})
			client := testClient(t, rt)
			post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p"})
			if err != nil {
				t.Fatalf("Post: %v", err)
			}
			if len(post.Body.Assets) != 1 {
				t.Fatalf("post assets = %+v", post.Body.Assets)
			}
			if got := post.Body.Assets[0].Resource.RequiresCredentials; got != tc.want {
				t.Fatalf("RequiresCredentials = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFanboxOpenResourceReResolvesFreshLocator 验证 finding #5：一个没有
// in-session locator 缓存的 ref（模拟跨进程或缓存淘汰）会被重新解析为最新
// locator，而不是重开过时的内嵌 URL。ref 通过 Post 取得，再用一个新的
// client 打开，强制走 re-resolution 路径。
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
	ref := post.Body.Assets[0].Resource.Ref

	// 用一个全新的 client 打开 ref：它的 locator 缓存为空，必须通过重新拉取
	// post.info 并按 attachment id 解析出最新 locator。
	consumer := testClient(t, rt)
	response, err := consumer.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: ref, Method: sdk.ResourceMethodGet})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer response.Body.Close()
	if mediaSeen != originalURL {
		t.Fatalf("re-resolved media URL = %q, want %q", mediaSeen, originalURL)
	}
}

// TestFanboxOpenResourceRejectsRefWithForeignProduct 验证 ref 必须属于 fanbox
// product，跨 product 的 ref 被拒绝。
func TestFanboxOpenResourceRejectsRefWithForeignProduct(t *testing.T) {
	pixivRef, err := sdk.NewResourceRef("pixiv", []byte(`{"k":"artwork","id":42}`))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	client := testClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no network expected")
	}))
	_, err = client.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: pixivRef})
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected InvalidArgument for foreign product ref, got %v", err)
	}
}
