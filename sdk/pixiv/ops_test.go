package pixiv

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestSearchArtworksWiresOperation(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "app-api.pixiv.net" {
			return nil, errors.New("unexpected host " + req.URL.Host)
		}
		if req.URL.Path != "/v1/search/illust" {
			t.Errorf("path = %s", req.URL.Path)
		}
		offset := req.URL.Query().Get("offset")
		if calls == 1 && offset != "" {
			t.Errorf("first call should omit offset, got %q", offset)
		}
		if calls == 2 && offset != "30" {
			t.Errorf("second call should pass offset=30, got %q", offset)
		}
		body := `{"illusts":[{"id":9001,"title":"art","type":"illust","create_date":"2024-05-01T10:00:00+09:00","image_urls":{"original":"https://i.pximg.net/img/9001.png"},"user":{"id":7,"name":"n","account":"a"},"tags":[]}],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=test&search_target=partial_match_for_tags&offset=30"}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})

	page, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test", Target: SearchTargetPartialMatchForTags})
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 9001 {
		t.Fatalf("page items = %+v", page.Items)
	}
	if page.Next.IsZero() {
		t.Fatal("expected a continuation cursor")
	}
	// Continue pagination with the cursor; the query digest must match.
	page2, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test", Target: SearchTargetPartialMatchForTags, Cursor: page.Next})
	if err != nil {
		t.Fatalf("continuation SearchArtworks: %v", err)
	}
	if len(page2.Items) != 1 || calls != 2 {
		t.Fatalf("page2 items=%d calls=%d", len(page2.Items), calls)
	}
}

func TestSearchArtworksRejectsChangedQuery(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=test&offset=30"}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	page, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test"})
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if page.Next.IsZero() {
		t.Fatal("expected cursor")
	}
	// Reusing the cursor with a different query must fail closed.
	if _, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "different", Cursor: page.Next}); sdk.CodeOf(err) != sdk.CodeInvalidCursor {
		t.Fatalf("expected CodeInvalidCursor for changed query, got %v", err)
	}
}

func TestArtworkWiresDetail(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/illust/detail" {
			t.Errorf("path = %s", req.URL.Path)
		}
		body := `{"illust":{"id":5,"title":"one","type":"manga","create_date":"2024-01-01T00:00:00Z","page_count":2,"image_urls":{"original":"https://i.pximg.net/img/5.png"},"meta_pages":[{"page_index":0,"width":100,"height":100,"image_urls":{"original":"https://i.pximg.net/img/5_p0.png"}},{"page_index":1,"width":100,"height":100,"image_urls":{"original":"https://i.pximg.net/img/5_p1.png"}}],"user":{"id":9,"name":"u","account":"u"},"tags":[]}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	artwork, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 5})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	if artwork.ID != 5 || len(artwork.Pages) != 2 {
		t.Fatalf("artwork = %+v", artwork)
	}
}

func TestUgoiraMetadataWires(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/ugoira/metadata" {
			t.Errorf("path = %s", req.URL.Path)
		}
		body := `{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/zip/m.zip","original":"https://i.pximg.net/zip/o.zip"},"frames":[{"file":"0.jpg","delay":100}]}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	meta, err := client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 77})
	if err != nil {
		t.Fatalf("UgoiraMetadata: %v", err)
	}
	if meta.ArtworkID != 77 || len(meta.Archives) != 2 || len(meta.Frames) != 1 {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestNoWebFallbackOnUnauthorized(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "www.pixiv.net" {
			t.Fatal("must never fall back to the Web API")
		}
		if req.URL.Host == "app-api.pixiv.net" {
			return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return nil, errors.New("unexpected host " + req.URL.Host)
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	_, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test"})
	if sdk.CodeOf(err) != sdk.CodeCredentialsExpired {
		t.Fatalf("expected CodeCredentialsExpired, got %v", err)
	}
}

func TestAddBookmarkWiresMutation(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "app-api.pixiv.net" || req.URL.Path != "/v2/illust/bookmark/add" {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.String())
		}
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err := client.AddBookmark(context.Background(), AddBookmarkRequest{ArtworkID: 12, Tags: []string{"tag"}}); err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
}
