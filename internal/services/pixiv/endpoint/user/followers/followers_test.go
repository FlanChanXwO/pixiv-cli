package followers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/followers"
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
func TestFollowersMapsQuery(t *testing.T) {
	f := &fakeTransport{body: `{"user_previews":[{"user":{"id":81}}]}`}
	result, err := followers.New(f).List(context.Background(), followers.Request{UserID: 8, Restrict: "public"})
	if err != nil || f.path != "/v1/user/follower" || f.query.Get("user_id") != "8" || f.query.Get("restrict") != "public" || len(result.Items) != 1 || result.Items[0].User.ID != 81 {
		t.Fatalf("result=%#v request=%q %v err=%v", result, f.path, f.query, err)
	}
}

func TestFollowersValidatesRequiredListAndUserIDs(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"user_previews":null}`,
		`{"user_previews":[{"user":{"id":0}}]}`,
	} {
		_, err := followers.New(&fakeTransport{body: body}).List(context.Background(), followers.Request{UserID: 8, Restrict: "public"})
		if !errors.Is(err, protocol.ErrMalformedResponse) {
			t.Fatalf("body %s error = %v, want malformed response", body, err)
		}
	}
	result, err := followers.New(&fakeTransport{body: `{"user_previews":[]}`}).List(context.Background(), followers.Request{UserID: 8, Restrict: "public"})
	if err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("empty result = %#v", result)
	}
}
