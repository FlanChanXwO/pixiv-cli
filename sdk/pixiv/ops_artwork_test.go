package pixiv_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestSearchArtworksInitialOffsetBindsCursor(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		want := "270"
		if calls == 2 {
			want = "300"
		}
		if got := req.URL.Query().Get("offset"); got != want {
			t.Errorf("request %d offset = %q, want %q", calls, got, want)
		}
		return jsonResponse(`{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=test&offset=300"}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatal(err)
	}
	request := SearchArtworksRequest{Word: "test", Offset: 270}
	page, err := client.SearchArtworks(context.Background(), request)
	if err != nil || page.Next.IsZero() {
		t.Fatalf("first page = %+v, err = %v", page, err)
	}
	for _, offset := range []int{0, 300} {
		request.Offset = offset
		request.Cursor = page.Next
		if _, err := client.SearchArtworks(context.Background(), request); sdk.ReasonOf(err) != sdk.InvalidCursor {
			t.Fatalf("offset %d: want InvalidCursor, got %v", offset, err)
		}
	}
	if calls != 1 {
		t.Fatalf("mismatched cursors reached upstream: %d calls", calls)
	}
	request.Offset = 270
	page, err = client.SearchArtworks(context.Background(), request)
	if err != nil || calls != 2 {
		t.Fatalf("continuation page = %+v, calls = %d, err = %v", page, calls, err)
	}
}

func TestSearchArtworksRejectsNegativeInitialOffset(t *testing.T) {
	client, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test", Offset: -1}); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("want InvalidArgument for negative offset, got %v", err)
	}
}
