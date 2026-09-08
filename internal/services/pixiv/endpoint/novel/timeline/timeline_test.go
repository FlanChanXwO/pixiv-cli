package timeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/timeline"
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
		{name: "following", request: timeline.Request{Kind: timeline.Following, Restrict: "private", Offset: 20}, path: "/v1/novel/follow", query: map[string]string{"restrict": "private", "offset": "20"}},
		{name: "latest", request: timeline.Request{Kind: timeline.Latest, MaxNovelID: 987654}, path: "/v1/novel/new", query: map[string]string{"filter": "for_android", "max_novel_id": "987654"}},
		{name: "mypixiv", request: timeline.Request{Kind: timeline.MyPixiv, Offset: 40}, path: "/v1/novel/mypixiv", query: map[string]string{"offset": "40"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{body: `{"novels":[]}`}
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

func TestFollowingRejectsInvalidRequestBeforeTransport(t *testing.T) {
	tests := []struct {
		name    string
		request timeline.Request
	}{
		{name: "negative offset", request: timeline.Request{Kind: timeline.Following, Restrict: "public", Offset: -1}},
		{name: "unsupported restrict", request: timeline.Request{Kind: timeline.Following, Restrict: "friends"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{body: `{"novels":[]}`}
			_, err := timeline.New(transport).List(context.Background(), test.request)
			if err == nil {
				t.Fatal("invalid following request unexpectedly succeeded")
			}
			if transport.calls != 0 {
				t.Fatalf("invalid request reached transport %d time(s)", transport.calls)
			}
		})
	}
}

func TestTimelineRejectsNullNovelList(t *testing.T) {
	_, err := timeline.New(&fakeTransport{body: `{"novels":null}`}).List(context.Background(), timeline.Request{Kind: timeline.Latest})
	if err == nil {
		t.Fatal("null list unexpectedly succeeded")
	}
}

func TestMyPixivMapsNovelDTO(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[{"id":101,"title":"story","user":{"id":77,"name":"writer"},"create_date":"2024-01-02T03:04:05+00:00","tags":[{"name":"fantasy"}]}]}`}
	result, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.MyPixiv})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 101 || result.Items[0].Title != "story" || result.Items[0].User.ID != 77 || len(result.Items[0].Tags) != 1 || result.Items[0].Tags[0].Name != "fantasy" {
		t.Fatalf("result = %#v", result)
	}
}

func TestLatestNovelRejectsOffsetContinuationRequest(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[]}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest, Offset: 30})
	if err == nil {
		t.Fatal("offset continuation unexpectedly succeeded")
	}
	if transport.calls != 0 {
		t.Fatalf("invalid request reached transport %d time(s)", transport.calls)
	}
}

func TestLatestNovelPreservesMaxNovelIDContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/new?max_novel_id=987654"}`}
	result, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.NextKey != "max_novel_id" || result.NextValue != 987654 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestLatestNovelUsesMaxNovelIDContinuationRequest(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[]}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest, MaxNovelID: 987654})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := transport.query.Get("filter"); got != "for_android" {
		t.Fatalf("filter = %q, want for_android; query=%v", got, transport.query)
	}
	if got := transport.query.Get("max_novel_id"); got != "987654" {
		t.Fatalf("max_novel_id = %q, want 987654; query=%v", got, transport.query)
	}
	if _, present := transport.query["offset"]; present {
		t.Fatalf("target query unexpectedly contains offset: %v", transport.query)
	}
}

func TestLatestNovelRejectsNegativeMaxNovelID(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[]}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest, MaxNovelID: -1})
	if err == nil {
		t.Fatal("negative max_novel_id unexpectedly succeeded")
	}
	if transport.calls != 0 {
		t.Fatalf("invalid request reached transport %d time(s)", transport.calls)
	}
}

func TestLatestNovelRejectsOffsetContinuationResponse(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/new?offset=30"}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest})
	if !errors.Is(err, protocol.ErrMalformedResponse) {
		t.Fatalf("List error = %v, want malformed response", err)
	}
}

func TestLatestNovelRejectsMixedContinuationKeys(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/new?max_novel_id=987654&offset=30"}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.Latest})
	if !errors.Is(err, protocol.ErrMalformedResponse) {
		t.Fatalf("List error = %v, want malformed response", err)
	}
}

func TestMyPixivRejectsContinuationWithFollowingQueryKey(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/novel/follow?offset=20&restrict=public"}`}
	_, err := timeline.New(transport).List(context.Background(), timeline.Request{Kind: timeline.MyPixiv})
	if !errors.Is(err, protocol.ErrMalformedResponse) {
		t.Fatalf("List error = %v, want malformed response", err)
	}
}
