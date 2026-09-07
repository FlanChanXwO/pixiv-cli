package following_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/following"
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
func TestFollowingMapsQueryAndContinuation(t *testing.T) {
	f := &fakeTransport{body: `{"user_previews":[{"user":{"id":71}}],"next_url":"https://app-api.pixiv.net/v1/user/following?offset=20"}`}
	result, err := following.New(f).List(context.Background(), following.Request{UserID: 8, Restrict: "private", Offset: 5})
	if err != nil || f.path != "/v1/user/following" || f.query.Get("user_id") != "8" || f.query.Get("restrict") != "private" || f.query.Get("offset") != "5" || len(result.Items) != 1 || result.Items[0].User.ID != 71 || result.NextOffset != 20 || !result.HasNext {
		t.Fatalf("result=%#v request=%q %v err=%v", result, f.path, f.query, err)
	}
}

func TestFollowingValidatesRequiredListAndUserIDs(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"user_previews":null}`,
		`{"user_previews":[{"user":{"id":0}}]}`,
	} {
		_, err := following.New(&fakeTransport{body: body}).List(context.Background(), following.Request{UserID: 8, Restrict: "private"})
		if !errors.Is(err, protocol.ErrMalformedResponse) {
			t.Fatalf("body %s error = %v, want malformed response", body, err)
		}
	}
	result, err := following.New(&fakeTransport{body: `{"user_previews":[]}`}).List(context.Background(), following.Request{UserID: 8, Restrict: "private"})
	if err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("empty result = %#v", result)
	}
}
