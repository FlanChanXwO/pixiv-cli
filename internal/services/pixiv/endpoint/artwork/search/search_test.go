package search_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/search"
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

func TestSearchMapsRouteQueryAndNormalizedArtwork(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[{"id":9001,"title":"art","type":"illust","create_date":"2024-05-01T10:00:00+09:00","image_urls":{"original":"https://i.pximg.net/img/9001.png"},"user":{"id":7,"name":"artist","account":"artist"},"tags":[]}],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=landscape&offset=30"}`}
	client := search.New(transport)

	result, err := client.Search(context.Background(), search.Request{
		Word:   "landscape",
		Target: "partial_match_for_tags",
		Sort:   "date_desc",
		Filters: search.Filters{
			ContentType: "illust",
			AIMode:      "exclude",
			Resolution:  "high",
		},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if transport.path != "/v1/search/illust" {
		t.Fatalf("path = %q, want /v1/search/illust", transport.path)
	}
	if got := transport.query.Get("word"); got != "landscape" {
		t.Fatalf("word = %q, want landscape", got)
	}
	if got := transport.query.Get("content_type"); got != "illust" {
		t.Fatalf("content_type = %q, want illust", got)
	}
	if got := transport.query.Get("search_ai_type"); got != "1" {
		t.Fatalf("search_ai_type = %q, want 1", got)
	}
	if got := transport.query.Get("width_min"); got != "3000" {
		t.Fatalf("width_min = %q, want 3000", got)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 9001 || result.Items[0].User.ID != 7 {
		t.Fatalf("items = %+v", result.Items)
	}
	if !result.HasNext || result.NextOffset != 30 {
		t.Fatalf("continuation = has_next:%v offset:%d, want true/30", result.HasNext, result.NextOffset)
	}
}

func TestSearchPreservesSuccessfulEmptyListAndRejectsNullList(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "empty array", body: `{"illusts":[],"next_url":null}`},
		{name: "null list", body: `{"illusts":null}`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{body: test.body}
			result, err := search.New(transport).Search(context.Background(), search.Request{Word: "empty"})
			if test.wantErr {
				if err == nil {
					t.Fatal("Search returned nil error for null list")
				}
				return
			}
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if result.Items == nil {
				t.Fatal("successful empty list must have non-nil Items")
			}
			if result.HasNext {
				t.Fatal("terminal empty list must not have continuation")
			}
		})
	}
}
