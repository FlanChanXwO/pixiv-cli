package timeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/timeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type fakeTransport struct {
	path  string
	query url.Values
	body  string
	calls int
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.calls++
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
		{name: "latest", request: timeline.Request{Kind: timeline.Latest, ContentType: "manga"}, path: "/v1/illust/new", query: map[string]string{"content_type": "manga", "filter": "for_android"}},
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

func TestUserArtworksNormalizesIllustrationAndRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name    string
		request timeline.Request
		want    string
	}{
		{name: "default subtype", request: timeline.Request{Kind: timeline.UserArtworks, UserID: 77}, want: "illust"},
		{name: "legacy illustration spelling", request: timeline.Request{Kind: timeline.UserArtworks, UserID: 77, ArtworkType: "illustration"}, want: "illust"},
		{name: "zero user id", request: timeline.Request{Kind: timeline.UserArtworks, UserID: 0, ArtworkType: "manga"}},
		{name: "negative user id", request: timeline.Request{Kind: timeline.UserArtworks, UserID: -1, ArtworkType: "manga"}},
		{name: "unsupported subtype", request: timeline.Request{Kind: timeline.UserArtworks, UserID: 77, ArtworkType: "all"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{body: `{"illusts":[]}`}
			_, err := timeline.New(transport).List(context.Background(), test.request)
			if test.want == "" {
				if err == nil {
					t.Fatal("invalid user-artworks request unexpectedly succeeded")
				}
				if transport.calls != 0 {
					t.Fatalf("invalid request reached transport %d time(s)", transport.calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if got := transport.query.Get("type"); got != test.want {
				t.Fatalf("type = %q, want %q; query=%v", got, test.want, transport.query)
			}
		})
	}
}

func TestUserArtworksMapsArtworkDTO(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[{"id":101,"title":"art","type":"illust","user":{"id":77,"name":"artist"},"tags":[{"name":"tag"}]}]}`}
	result, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.UserArtworks, UserID: 77, ArtworkType: "illust"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 101 || result.Items[0].Title != "art" || result.Items[0].User.ID != 77 || len(result.Items[0].Tags) != 1 || result.Items[0].Tags[0].Name != "tag" {
		t.Fatalf("result = %#v", result)
	}
}

func TestMyPixivRejectsNonPositiveNestedOwnerID(t *testing.T) {
	tests := []struct {
		name    string
		ownerID string
	}{
		{name: "zero", ownerID: "0"},
		{name: "negative", ownerID: "-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{body: `{"illusts":[{"id":101,"user":{"id":` + test.ownerID + `}}]}`}
			result, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.MyPixiv})
			if !errors.Is(err, protocol.ErrMalformedResponse) {
				t.Fatalf("List error = %v, want malformed response", err)
			}
			if result.Items != nil {
				t.Fatalf("result contains partial items: %#v", result.Items)
			}
		})
	}
}

func TestMyPixivMapsArtworkDTO(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[{"id":101,"title":"art","type":"illust","user":{"id":77,"name":"artist"},"tags":[{"name":"tag"}]}]}`}
	result, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.MyPixiv})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 101 || result.Items[0].Title != "art" || result.Items[0].User.ID != 77 || len(result.Items[0].Tags) != 1 || result.Items[0].Tags[0].Name != "tag" {
		t.Fatalf("result = %#v", result)
	}
}

func TestLatestTimelinePreservesMaxIllustIDContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/illust/new?max_illust_id=987654"}`}
	result, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest, ContentType: "illust"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.NextKey != "max_illust_id" || result.NextValue != 987654 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestLatestTimelineRejectsDuplicateContinuationValues(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/illust/new?max_illust_id=987654&max_illust_id=123456&offset=30"}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest, ContentType: "illust"})
	if err == nil {
		t.Fatal("duplicate continuation values unexpectedly succeeded")
	}
}

func TestLatestTimelineDefaultsContentTypeAndSendsFilter(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[]}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := transport.query.Get("content_type"); got != "illust" {
		t.Fatalf("content_type = %q, want illust; query=%v", got, transport.query)
	}
	if got := transport.query.Get("filter"); got != "for_android" {
		t.Fatalf("filter = %q, want for_android; query=%v", got, transport.query)
	}
	if _, present := transport.query["max_illust_id"]; present {
		t.Fatalf("initial query unexpectedly contains max_illust_id: %v", transport.query)
	}
}

func TestLatestTimelineRejectsUnapprovedContentType(t *testing.T) {
	for _, contentType := range []string{"ugoira", "all", "illust-and-ugoira"} {
		t.Run(contentType, func(t *testing.T) {
			transport := &fakeTransport{body: `{"illusts":[]}`}
			_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest, ContentType: contentType})
			if err == nil {
				t.Fatalf("content type %q unexpectedly succeeded", contentType)
			}
			if transport.calls != 0 {
				t.Fatalf("invalid request reached transport %d time(s)", transport.calls)
			}
		})
	}
}

func TestLatestTimelineRejectsOffsetContinuationRequest(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[]}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest, ContentType: "illust", Offset: 30})
	if err == nil {
		t.Fatal("offset continuation unexpectedly succeeded")
	}
	if transport.calls != 0 {
		t.Fatalf("invalid request reached transport %d time(s)", transport.calls)
	}
}

func TestLatestTimelineRejectsNegativeMaxIllustID(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[]}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest, ContentType: "illust", MaxIllustID: -1})
	if err == nil {
		t.Fatal("negative max_illust_id unexpectedly succeeded")
	}
	if transport.calls != 0 {
		t.Fatalf("invalid request reached transport %d time(s)", transport.calls)
	}
}

func TestLatestTimelineUsesMaxIllustIDContinuationRequest(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[]}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest, ContentType: "illust", MaxIllustID: 987654})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := transport.query.Get("max_illust_id"); got != "987654" {
		t.Fatalf("max_illust_id = %q, want 987654; query=%v", got, transport.query)
	}
	if _, present := transport.query["offset"]; present {
		t.Fatalf("target query unexpectedly contains offset: %v", transport.query)
	}
}
