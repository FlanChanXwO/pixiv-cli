package timeline_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/timeline"
)

type fakeTransport struct {
	path  string
	query url.Values
	body  string
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.path = path
	f.query = query
	return json.Unmarshal([]byte(f.body), out)
}

func TestTimelineMapsConfirmedRoutesAndQueries(t *testing.T) {
	tests := []struct {
		name    string
		request timeline.Request
		path    string
		query   map[string]string
	}{
		{name: "following", request: timeline.Request{Kind: timeline.Following, Restrict: "private", Offset: 20}, path: "/v2/illust/follow", query: map[string]string{"restrict": "private", "offset": "20"}},
		{name: "latest", request: timeline.Request{Kind: timeline.Latest, ContentType: "manga", Offset: 30}, path: "/v1/illust/new", query: map[string]string{"content_type": "manga", "filter": "for_android", "offset": "30"}},
		{name: "mypixiv", request: timeline.Request{Kind: timeline.MyPixiv, Offset: 40}, path: "/v2/illust/mypixiv", query: map[string]string{"offset": "40"}},
		{name: "user", request: timeline.Request{Kind: timeline.UserArtworks, UserID: 77, ArtworkType: "manga", Offset: 50}, path: "/v1/user/illusts", query: map[string]string{"user_id": "77", "type": "manga", "offset": "50"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{body: `{"illusts":[]}`}
			result, err := timeline.New(transport).List(context.Background(), test.request)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if transport.path != test.path {
				t.Fatalf("path = %q, want %q", transport.path, test.path)
			}
			for key, want := range test.query {
				if got := transport.query.Get(key); got != want {
					t.Fatalf("%s = %q, want %q; query=%v", key, got, want, transport.query)
				}
			}
			if result.Items == nil {
				t.Fatal("empty array mapped to nil items")
			}
		})
	}
}

func TestTimelineRejectsNullList(t *testing.T) {
	_, err := timeline.New(&fakeTransport{body: `{"illusts":null}`}).List(context.Background(), timeline.Request{Kind: timeline.Latest, ContentType: "illust"})
	if err == nil {
		t.Fatal("null list unexpectedly succeeded")
	}
}
