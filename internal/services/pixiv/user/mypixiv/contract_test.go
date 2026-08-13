package mypixiv_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/user/mypixiv"
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
func TestMyPixivMapsUserIDFilterAndOffset(t *testing.T) {
	f := &fakeTransport{body: `{"user_previews":[{"user":{"id":101}}]}`}
	result, err := mypixiv.New(f).List(context.Background(), mypixiv.Request{UserID: 10, Offset: 6})
	if err != nil || f.path != "/v1/user/mypixiv" || f.query.Get("user_id") != "10" || f.query.Get("filter") != "for_android" || f.query.Get("offset") != "6" || len(result.Items) != 1 || result.Items[0].User.ID != 101 {
		t.Fatalf("result=%#v request=%q %v err=%v", result, f.path, f.query, err)
	}
}
