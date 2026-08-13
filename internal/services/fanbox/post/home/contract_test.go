package home_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/post/home"
)

type jsonTransport struct{ body string }

func (t *jsonTransport) GetJSON(_ context.Context, _ string, target any) error {
	return json.Unmarshal([]byte(t.body), target)
}

func TestListDecodesItemsEnvelope(t *testing.T) {
	transport := &jsonTransport{body: `{"body":{"items":[{"id":"home-1","title":"home"}],"nextUrl":"https://api.fanbox.cc/post.listHome?cursor=2"}}`}
	page, err := home.New(transport).List(context.Background(), home.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Posts) != 1 || page.Posts[0].ID != "home-1" || page.NextURL == "" {
		t.Fatalf("page = %+v", page)
	}
}
