package following_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/user/following"
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
