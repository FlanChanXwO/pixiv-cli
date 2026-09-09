package pixiv_test

import (
	"context"
	"net/http"
	"testing"

	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestArtworkDetailDerivesPageIndexesFromMetaPageOrder(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/illust/detail" {
			t.Errorf("path = %s", req.URL.Path)
		}
		// Pixiv 的真实 meta_pages 以数组顺序表达页序，不保证携带 page_index。
		body := `{"illust":{"id":5,"title":"one","type":"manga","create_date":"2024-01-01T00:00:00Z","page_count":2,"image_urls":{"original":"https://i.pximg.net/img/5.png"},"meta_pages":[{"width":100,"height":100,"image_urls":{"original":"https://i.pximg.net/img/5_p0.png"}},{"width":100,"height":100,"image_urls":{"original":"https://i.pximg.net/img/5_p1.png"}}],"user":{"id":9,"name":"u","account":"u"},"tags":[]}}`
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	artwork, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 5})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	if len(artwork.Pages) != 2 {
		t.Fatalf("page count = %d, want 2", len(artwork.Pages))
	}
	if artwork.Pages[0].PageIndex != 0 || artwork.Pages[1].PageIndex != 1 {
		t.Fatalf("page indexes = [%d %d], want [0 1]", artwork.Pages[0].PageIndex, artwork.Pages[1].PageIndex)
	}
	if artwork.Pages[0].Image.Resource.Ref.String() == artwork.Pages[1].Image.Resource.Ref.String() {
		t.Fatal("distinct artwork pages must not share the same resource ref")
	}
	if artwork.Pages[0].Image.Resource.URL != "https://i.pximg.net/img/5_p0.png" || artwork.Pages[1].Image.Resource.URL != "https://i.pximg.net/img/5_p1.png" {
		t.Fatalf("page URLs = [%q %q]", artwork.Pages[0].Image.Resource.URL, artwork.Pages[1].Image.Resource.URL)
	}
}
