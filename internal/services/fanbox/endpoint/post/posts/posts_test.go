package posts_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/posts"
)

type jsonTransport struct {
	endpoint string
	body     string
}

func (t *jsonTransport) GetJSON(_ context.Context, endpoint string, target any) error {
	t.endpoint = endpoint
	return json.Unmarshal([]byte(t.body), target)
}

func TestCreatorAddsUpstreamLimitAndTagUsesCreatorQuery(t *testing.T) {
	transport := &jsonTransport{body: `{"body":{"posts":[{"id":"post-1","title":"title"}]}}`}
	page, err := posts.New(transport).Creator(context.Background(), posts.Request{CreatorID: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Posts) != 1 || page.Posts[0].ID != "post-1" {
		t.Fatalf("page = %+v", page)
	}
	parsed, err := url.Parse(transport.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/post.listCreator" || parsed.Query().Get("creatorId") != "writer" || parsed.Query().Get("limit") != "10" {
		t.Fatalf("creator endpoint = %q", transport.endpoint)
	}

	transport.body = `{"body":{"posts":[{"id":"tagged","title":"tagged"}]}}`
	if _, err := posts.New(transport).Tagged(context.Background(), posts.Request{CreatorID: "writer", Tag: "fanart"}); err != nil {
		t.Fatal(err)
	}
	parsed, err = url.Parse(transport.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/post.listTagged" || parsed.Query().Get("creatorId") != "writer" || parsed.Query().Get("tag") != "fanart" {
		t.Fatalf("tagged endpoint = %q", transport.endpoint)
	}
}
