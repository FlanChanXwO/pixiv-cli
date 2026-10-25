package fanbox_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

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
