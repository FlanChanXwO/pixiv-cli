package related_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/related"
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
