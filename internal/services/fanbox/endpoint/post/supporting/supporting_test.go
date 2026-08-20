package supporting_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/supporting"
)

type jsonTransport struct {
	endpoint string
	body     string
}

func (t *jsonTransport) GetJSON(_ context.Context, endpoint string, target any) error {
	t.endpoint = endpoint
	return json.Unmarshal([]byte(t.body), target)
}

func TestListUsesSupportingRouteAndItemsEnvelope(t *testing.T) {
	transport := &jsonTransport{body: `{"body":{"items":[{"id":"support-1","title":"support"}]}}`}
	page, err := supporting.New(transport).List(context.Background(), supporting.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Posts) != 1 || page.Posts[0].ID != "support-1" {
		t.Fatalf("page = %+v", page)
	}
	parsed, err := url.Parse(transport.endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/post.listSupporting" || parsed.Query().Get("limit") != "10" {
		t.Fatalf("endpoint = %q", transport.endpoint)
	}
}
