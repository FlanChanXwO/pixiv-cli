package ranking_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/ranking"
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

func TestRankingMapsRouteQueryArtworkAndContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[{"id":5,"title":"ranked","user":{"id":6},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/illust/ranking?mode=day&date=2024-01-01&offset=30"}`}
	result, err := ranking.New(transport).List(context.Background(), ranking.Request{Mode: "day", Date: "2024-01-01", Offset: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v1/illust/ranking" || transport.query.Get("mode") != "day" || transport.query.Get("date") != "2024-01-01" || transport.query.Get("offset") != "10" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 5 || result.NextOffset != 30 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestRankingRejectsNullList(t *testing.T) {
	_, err := ranking.New(&fakeTransport{body: `{"illusts":null}`}).List(context.Background(), ranking.Request{Mode: "day"})
	if err == nil {
		t.Fatal("null ranking list unexpectedly succeeded")
	}
}

func TestRankingDefaultsModeToDay(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[],"next_url":null}`}
	if _, err := ranking.New(transport).List(context.Background(), ranking.Request{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := transport.query.Get("mode"); got != "day" {
		t.Fatalf("mode = %q, want %q", got, "day")
	}
}

func TestRankingRejectsInvalidRequestBeforeTransport(t *testing.T) {
	tests := []struct {
		name    string
		request ranking.Request
	}{
		{name: "unsupported mode", request: ranking.Request{Mode: "unknown"}},
		{name: "invalid date", request: ranking.Request{Mode: "day", Date: "2024-02-30"}},
		{name: "negative offset", request: ranking.Request{Mode: "day", Offset: -1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{body: `{"illusts":[],"next_url":null}`}
			if _, err := ranking.New(transport).List(context.Background(), test.request); err == nil {
				t.Fatal("invalid request unexpectedly succeeded")
			}
			if transport.calls != 0 {
				t.Fatalf("transport calls = %d, want 0", transport.calls)
			}
		})
	}
}

func TestRankingAcceptsFrozenModes(t *testing.T) {
	for _, mode := range []string{
		"day", "day_male", "day_female", "week", "week_original", "week_rookie", "month",
		"day_manga", "week_manga", "month_manga", "week_rookie_manga", "day_r18",
		"day_male_r18", "day_female_r18", "week_r18", "week_r18g",
	} {
		t.Run(mode, func(t *testing.T) {
			transport := &fakeTransport{body: `{"illusts":[],"next_url":null}`}
			if _, err := ranking.New(transport).List(context.Background(), ranking.Request{Mode: mode}); err != nil {
				t.Fatalf("List: %v", err)
			}
			if transport.query.Get("mode") != mode {
				t.Fatalf("mode query = %q, want %q", transport.query.Get("mode"), mode)
			}
		})
	}
}
