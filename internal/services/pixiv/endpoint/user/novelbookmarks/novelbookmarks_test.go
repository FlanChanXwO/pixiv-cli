package novelbookmarks_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/novelbookmarks"
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

func TestListMapsBookmarkQueryAndContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[{"id":4,"title":"saved","user":{"id":5},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?max_bookmark_id=8"}`}
	result, err := novelbookmarks.New(transport).List(context.Background(), novelbookmarks.Request{UserID: 7, Restrict: "private", Tag: "cat", MaxBookmarkID: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v1/user/bookmarks/novel" || transport.query.Get("user_id") != "7" || transport.query.Get("restrict") != "private" || transport.query.Get("tag") != "cat" || transport.query.Get("max_bookmark_id") != "3" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 4 || result.NextMaxBookmarkID != 8 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestListRejectsInvalidBookmarkContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?max_bookmark_id=0"}`}
	if _, err := novelbookmarks.New(transport).List(context.Background(), novelbookmarks.Request{UserID: 7, Restrict: "public"}); err == nil {
		t.Fatal("invalid continuation unexpectedly succeeded")
	}
}
