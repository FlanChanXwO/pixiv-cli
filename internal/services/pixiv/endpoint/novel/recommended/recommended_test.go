package recommended_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/recommended"
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

func TestRecommendedPreservesInitialAndContinuationOffset(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[{"id":31,"title":"recommended","user":{"id":7},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?offset=0"}`}
	client := recommended.New(transport)
	result, err := client.List(context.Background(), recommended.Request{})
	if err != nil {
		t.Fatalf("initial List: %v", err)
	}
	if transport.path != "/v1/novel/recommended" || transport.query.Get("offset") != "" || len(result.Items) != 1 || result.NextOffset != 0 || !result.HasNext {
		t.Fatalf("initial result=%#v request=%q %v", result, transport.path, transport.query)
	}
	result, err = client.List(context.Background(), recommended.Request{Offset: 0, ContinuationExists: true})
	if err != nil {
		t.Fatalf("continuation List: %v", err)
	}
	if transport.query.Get("offset") != "0" {
		t.Fatalf("continuation query = %v, want offset=0", transport.query)
	}
}

func TestRecommendedRejectsNullNovelList(t *testing.T) {
	_, err := recommended.New(&fakeTransport{body: `{"novels":null}`}).List(context.Background(), recommended.Request{})
	if err == nil {
		t.Fatal("null list unexpectedly succeeded")
	}
}
