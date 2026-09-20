package ranking_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/ranking"
)

type fakeTransport struct {
	path    string
	queries []url.Values
	bodies  []string
	call    int
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.path = path
	f.queries = append(f.queries, query)
	body := f.bodies[f.call]
	f.call++
	return json.Unmarshal([]byte(body), out)
}

func TestRankingMapsRouteQueryNovelAndContinuation(t *testing.T) {
	transport := &fakeTransport{bodies: []string{
		`{"novels":[{"id":11,"title":"first","text_length":42,"is_original":true,"user":{"id":7},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/novel/ranking?filter=for_android&mode=day&offset=30"}`,
		`{"novels":[{"id":12,"title":"second","user":{"id":8},"create_date":"2024-01-03T03:04:05+00:00"}],"next_url":null}`,
	}}
	client := ranking.New(transport)

	first, err := client.List(context.Background(), ranking.Request{})
	if err != nil {
		t.Fatalf("initial List: %v", err)
	}
	if transport.path != "/v1/novel/ranking" || len(transport.queries) != 1 {
		t.Fatalf("initial request = %q %v", transport.path, transport.queries)
	}
	if got := transport.queries[0]; got.Get("filter") != "for_android" || got.Get("mode") != "day" || got.Get("offset") != "" {
		t.Fatalf("initial query = %v", got)
	}
	if len(first.Items) != 1 || first.Items[0].ID != 11 || first.Items[0].User.ID != 7 || first.Items[0].TextLength != 42 || !first.Items[0].IsOriginal || first.NextOffset != 30 || !first.HasNext {
		t.Fatalf("initial result = %#v", first)
	}

	second, err := client.List(context.Background(), ranking.Request{Filter: "for_android", Mode: "day", Offset: first.NextOffset})
	if err != nil {
		t.Fatalf("continuation List: %v", err)
	}
	if got := transport.queries[1]; got.Get("filter") != "for_android" || got.Get("mode") != "day" || got.Get("offset") != "30" {
		t.Fatalf("continuation query = %v", got)
	}
	if len(second.Items) != 1 || second.Items[0].ID != 12 || second.HasNext {
		t.Fatalf("continuation result = %#v", second)
	}
}

func TestRankingPreservesFilterAndMode(t *testing.T) {
	transport := &fakeTransport{bodies: []string{`{"novels":[],"next_url":null}`}}
	_, err := ranking.New(transport).List(context.Background(), ranking.Request{Filter: "for_android", Mode: "week"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := transport.queries[0]; got.Get("filter") != "for_android" || got.Get("mode") != "week" {
		t.Fatalf("query = %v", got)
	}
}

func TestRankingRejectsMissingOrNullNovelList(t *testing.T) {
	for _, body := range []string{`{}`, `{"novels":null}`} {
		transport := &fakeTransport{bodies: []string{body}}
		_, err := ranking.New(transport).List(context.Background(), ranking.Request{})
		if err == nil {
			t.Fatalf("body %s unexpectedly succeeded", body)
		}
	}
}

func TestRankingAcceptsEmptyNovelList(t *testing.T) {
	transport := &fakeTransport{bodies: []string{`{"novels":[],"next_url":null}`}}
	result, err := ranking.New(transport).List(context.Background(), ranking.Request{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Items == nil || len(result.Items) != 0 || result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestRankingRejectsNegativeOffsetBeforeTransport(t *testing.T) {
	transport := &fakeTransport{bodies: []string{`{"novels":[]}`}}
	_, err := ranking.New(transport).List(context.Background(), ranking.Request{Offset: -1})
	if err == nil {
		t.Fatal("negative offset unexpectedly succeeded")
	}
	if transport.call != 0 {
		t.Fatalf("transport calls = %d, want 0", transport.call)
	}
}

func TestRankingRejectsNonPositiveContinuation(t *testing.T) {
	for _, nextURL := range []string{
		"https://app-api.pixiv.net/v1/novel/ranking?offset=0",
		"https://app-api.pixiv.net/v1/novel/ranking?offset=-1",
		"https://app-api.pixiv.net/v1/novel/ranking",
	} {
		t.Run(nextURL, func(t *testing.T) {
			transport := &fakeTransport{bodies: []string{`{"novels":[],"next_url":"` + nextURL + `"}`}}
			_, err := ranking.New(transport).List(context.Background(), ranking.Request{})
			if err == nil {
				t.Fatal("invalid continuation unexpectedly succeeded")
			}
		})
	}
}
