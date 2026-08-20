package novels_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/novels"
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

func TestListMapsUserFilterAndOffsetContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[{"id":4,"title":"story","user":{"id":5},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/user/novels?offset=8"}`}
	result, err := novels.New(transport).List(context.Background(), novels.Request{UserID: 7, Offset: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v1/user/novels" || transport.query.Get("user_id") != "7" || transport.query.Get("filter") != "for_android" || transport.query.Get("offset") != "3" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 4 || result.NextOffset != 8 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestListRejectsMissingEnvelopeAndInvalidContinuation(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"novels":null}`,
		`{"novels":[{"id":0,"user":{"id":5}}]}`,
		`{"novels":[],"next_url":"https://app-api.pixiv.net/v1/user/novels?offset=0"}`,
	} {
		transport := &fakeTransport{body: body}
		if _, err := novels.New(transport).List(context.Background(), novels.Request{UserID: 7}); err == nil {
			t.Fatalf("body %s unexpectedly succeeded", body)
		}
	}
}
