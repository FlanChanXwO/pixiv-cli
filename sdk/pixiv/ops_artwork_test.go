package pixiv_test

import (
	"context"
	"net/http"
	"testing"

	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestRecommendedArtworksPreservesExplicitZeroContinuation(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/illust/recommended" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if calls == 1 {
			if _, ok := query["offset"]; ok {
				t.Errorf("initial request must not include offset: %v", query)
			}
		} else if query.Get("offset") != "0" {
			t.Errorf("continuation offset = %q, want %q", query.Get("offset"), "0")
		}
		body := `{"illusts":[{"id":7301,"title":"recommended artwork","type":"illust","create_date":"2026-01-05T00:00:00Z","user":{"id":21,"name":"artist"}}],"next_url":"https://app-api.pixiv.net/v1/illust/recommended?offset=0"}`
		if calls == 2 {
			body = `{"illusts":[],"next_url":null}`
		}
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := RecommendedArtworksRequest{}
	page, err := client.RecommendedArtworks(context.Background(), request)
	if err != nil {
		t.Fatalf("RecommendedArtworks: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 7301 || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.RecommendedArtworks(context.Background(), request)
	if err != nil {
		t.Fatalf("RecommendedArtworks continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}
