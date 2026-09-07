package related_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/related"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type fakeTransport struct {
	path  string
	query url.Values
	body  string
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.path, f.query = path, query
	return json.Unmarshal([]byte(f.body), out)
}
func TestRelatedMapsSeedAndContinuation(t *testing.T) {
	f := &fakeTransport{body: `{"user_previews":[{"user":{"id":61,"name":"artist"}}],"next_url":"https://app-api.pixiv.net/v1/user/related?offset=20"}`}
	result, err := related.New(f).List(context.Background(), related.Request{SeedUserID: 9, Offset: 5})
	if err != nil || f.path != "/v1/user/related" || f.query.Get("seed_user_id") != "9" || f.query.Get("offset") != "5" || len(result.Items) != 1 || result.Items[0].User.ID != 61 || result.NextOffset != 20 || !result.HasNext {
		t.Fatalf("result=%#v request=%q %v err=%v", result, f.path, f.query, err)
	}
}
func TestRelatedRejectsNullList(t *testing.T) {
	if _, err := related.New(&fakeTransport{body: `{"user_previews":null}`}).List(context.Background(), related.Request{SeedUserID: 1}); !errors.Is(err, protocol.ErrMalformedResponse) {
		t.Fatalf("null list error = %v, want malformed response", err)
	}
}

func TestRelatedAcceptsEmptyListAndRejectsInvalidUser(t *testing.T) {
	result, err := related.New(&fakeTransport{body: `{"user_previews":[]}`}).List(context.Background(), related.Request{SeedUserID: 1})
	if err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if result.Items == nil || len(result.Items) != 0 || result.HasNext {
		t.Fatalf("empty result = %#v", result)
	}

	_, err = related.New(&fakeTransport{body: `{"user_previews":[{"user":{"id":0}}]}`}).List(context.Background(), related.Request{SeedUserID: 1})
	if !errors.Is(err, protocol.ErrMalformedResponse) {
		t.Fatalf("invalid user error = %v, want malformed response", err)
	}
}
