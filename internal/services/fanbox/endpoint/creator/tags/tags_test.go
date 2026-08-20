package tags_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator/tags"
)

type jsonTransport struct {
	endpoint string
	body     string
}

func (t *jsonTransport) GetJSON(_ context.Context, endpoint string, target any) error {
	t.endpoint = endpoint
	return json.Unmarshal([]byte(t.body), target)
}

func TestListMapsFeaturedTagsAndQuery(t *testing.T) {
	transport := &jsonTransport{body: `{"body":{"tags":[{"tag":"fanart","url":"https://www.fanbox.cc/@writer/posts/tag/fanart"}]}}`}
	items, err := tags.New(transport).List(context.Background(), tags.Request{CreatorID: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "fanart" || items[0].URL == "" {
		t.Fatalf("items = %+v", items)
	}
	parsed, err := url.Parse(transport.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/tag.getFeatured" || parsed.Query().Get("creatorId") != "writer" {
		t.Fatalf("endpoint = %q", transport.endpoint)
	}
}
