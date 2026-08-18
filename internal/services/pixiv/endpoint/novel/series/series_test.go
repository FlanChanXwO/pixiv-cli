package series_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/series"
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
	transport := &fakeTransport{body: `{"novel_series_detail":{"id":21,"title":"series","user":{"id":7}},"novels":[{"id":22,"title":"chapter","user":{"id":7},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/novel/series?series_id=21&last_order=9"}`}
	result, err := series.New(transport).List(context.Background(), series.Request{SeriesID: 21, LastOrder: 4})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v1/novel/series" || transport.query.Get("series_id") != "21" || transport.query.Get("last_order") != "4" || result.Series.ID != 21 || len(result.Items) != 1 || result.Items[0].ID != 22 || result.NextLastOrder != 9 || !result.HasNext {
		t.Fatalf("result = %#v request=%q %v", result, transport.path, transport.query)
	}
}

func TestSeriesRejectsMissingDetailOrInvalidNovel(t *testing.T) {
	for _, body := range []string{`{"novels":[]}`, `{"novel_series_detail":null,"novels":[]}`, `{"novel_series_detail":{"id":1,"user":{"id":1}},"novels":[{"id":0,"user":{"id":1}}]}`} {
		_, err := series.New(&fakeTransport{body: body}).List(context.Background(), series.Request{SeriesID: 1})
		if err == nil {
			t.Fatalf("body %s unexpectedly succeeded", body)
		}
	}
}
