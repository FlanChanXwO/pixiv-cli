package ranking_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/ranking"
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
