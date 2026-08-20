package trending_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/trending"
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

func TestTrendingMapsCompleteListAndArtwork(t *testing.T) {
	transport := &fakeTransport{body: `{"trend_tags":[{"tag":"cat","translated_name":"猫","illust":{"id":1,"title":"cover","user":{"id":2},"create_date":"2024-01-02T03:04:05+00:00"}},{"tag":"dog","illust":{"id":3,"user":{"id":4},"create_date":"2024-01-03T03:04:05+00:00"}}]}`}
	result, err := trending.New(transport).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v1/trending-tags/illust" || len(transport.query) != 0 {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result) != 2 || result[0].Tag != "cat" || result[1].Artwork.ID != 3 {
		t.Fatalf("result = %#v", result)
	}
}

func TestTrendingRejectsMissingTagArtwork(t *testing.T) {
	_, err := trending.New(&fakeTransport{body: `{"trend_tags":[{"tag":"cat","illust":null}]}`}).List(context.Background())
	if err == nil {
		t.Fatal("missing trend artwork unexpectedly succeeded")
	}
}
