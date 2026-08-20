package blocked_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/blocked"
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
func TestBlockedMapsUserAndOffset(t *testing.T) {
	f := &fakeTransport{body: `{"user_previews":[{"user":{"id":91}}]}`}
	result, err := blocked.New(f).List(context.Background(), blocked.Request{UserID: 9, Offset: 4})
	if err != nil || f.path != "/v1/user/list" || f.query.Get("user_id") != "9" || f.query.Get("offset") != "4" || len(result.Items) != 1 || result.Items[0].User.ID != 91 {
		t.Fatalf("result=%#v request=%q %v err=%v", result, f.path, f.query, err)
	}
}
