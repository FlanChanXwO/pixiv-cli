package search_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/novel/search"
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

func TestSearchMapsRouteQueryNovelAndContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[{"id":11,"title":"story","caption":"body","x_restrict":0,"text_length":42,"is_original":true,"create_date":"2024-01-02T03:04:05+00:00","user":{"id":7,"name":"writer"},"tags":[{"name":"fantasy","translated_name":"幻想"}],"image_urls":{"original":"https://i.example/novel.jpg"}}],"next_url":"https://app-api.pixiv.net/v1/search/novel?word=story&offset=20"}`}
	result, err := search.New(transport).Search(context.Background(), search.Request{
		Word: "story", Target: "partial_match_for_tags", Sort: "date_desc", Duration: "within_last_week", Offset: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if transport.path != "/v1/search/novel" || transport.query.Get("word") != "story" || transport.query.Get("search_target") != "partial_match_for_tags" || transport.query.Get("sort") != "date_desc" || transport.query.Get("duration") != "within_last_week" || transport.query.Get("offset") != "5" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 11 || result.Items[0].User.ID != 7 || result.Items[0].TextLength != 42 || !result.Items[0].IsOriginal || result.NextOffset != 20 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestSearchRejectsMissingOrNullNovelList(t *testing.T) {
	for _, body := range []string{`{}`, `{"novels":null}`} {
		_, err := search.New(&fakeTransport{body: body}).Search(context.Background(), search.Request{Word: "story"})
		if err == nil {
			t.Fatalf("body %s unexpectedly succeeded", body)
		}
	}
}
