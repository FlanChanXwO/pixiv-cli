package series_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/series"
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

func TestSeriesMapsRouteQueryAndContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"illust_series_detail":{"user":{"id":7}},"illusts":[{"id":456,"title":"episode","user":{"id":9},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/illust/series?illust_series_id=42&last_order=8"}`}
	result, err := series.New(transport).List(context.Background(), series.Request{SeriesID: 42, LastOrder: 4})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v1/illust/series" || transport.query.Get("illust_series_id") != "42" || transport.query.Get("last_order") != "4" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 456 || result.NextLastOrder != 8 || !result.HasNext || result.UserID != 7 {
		t.Fatalf("result = %#v", result)
	}
}

func TestSeriesRejectsMissingDetailUser(t *testing.T) {
	_, err := series.New(&fakeTransport{body: `{"illust_series_detail":null,"illusts":[]}`}).List(context.Background(), series.Request{SeriesID: 42})
	if err == nil {
		t.Fatal("missing series detail unexpectedly succeeded")
	}
}
