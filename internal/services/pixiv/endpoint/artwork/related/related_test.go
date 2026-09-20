package related_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/related"
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

func TestRelatedMapsRouteArtworkAndContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[{"id":456,"title":"related","user":{"id":9},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v2/illust/related?illust_id=123&offset=30"}`}
	result, err := related.New(transport).List(context.Background(), related.Request{ArtworkID: 123, Offset: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v2/illust/related" || transport.query.Get("illust_id") != "123" || transport.query.Get("offset") != "10" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 456 || result.NextOffset != 30 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestRelatedReplaysLiveMultiParamContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[{"id":456,"title":"related","user":{"id":9},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v2/illust/related?illust_id=123&seed_illust_ids%5B0%5D=456&seed_illust_ids%5B1%5D=789&viewed%5B0%5D=123&viewed%5B1%5D=456"}`}
	client := related.New(transport)
	first, err := client.List(context.Background(), related.Request{ArtworkID: 123})
	if err != nil {
		t.Fatalf("first List: %v", err)
	}
	if !first.HasNext || len(first.NextParams["seed_illust_ids[]"]) != 2 || first.NextParams["seed_illust_ids[]"][0] != "456" || first.NextParams["seed_illust_ids[]"][1] != "789" || len(first.NextParams["viewed[]"]) != 2 || first.NextParams["viewed[]"][1] != "456" {
		t.Fatalf("first result = %#v", first)
	}
	transport.body = `{"illusts":[],"next_url":null}`
	second, err := client.List(context.Background(), related.Request{ArtworkID: 123, ContinuationParams: first.NextParams})
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if got := transport.query; got.Get("illust_id") != "123" || len(got["seed_illust_ids[]"]) != 2 || got["seed_illust_ids[]"][1] != "789" || len(got["viewed[]"]) != 2 || got["viewed[]"][1] != "456" {
		t.Fatalf("continuation query = %v", got)
	}
	if second.Items == nil || len(second.Items) != 0 || second.HasNext {
		t.Fatalf("second result = %#v", second)
	}
}

func TestRelatedPreservesEmptyArrayAndRejectsNull(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		ok   bool
	}{
		{name: "empty", body: `{"illusts":[]}`, ok: true},
		{name: "null", body: `{"illusts":null}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := related.New(&fakeTransport{body: test.body}).List(context.Background(), related.Request{ArtworkID: 1})
			if test.ok {
				if err != nil || result.Items == nil {
					t.Fatalf("result=%#v err=%v", result, err)
				}
				return
			}
			if err == nil {
				t.Fatal("null list unexpectedly succeeded")
			}
		})
	}
}
