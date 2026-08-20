package comments_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/comments"
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

func TestCommentsMapsParentMetadataAndContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"comments":[{"id":8,"comment":"child","created_at":"2024-01-02T03:04:05+00:00","user":{"id":7},"parent_comment":{"id":6,"caption":"parent","created_at":"2024-01-01T03:04:05+00:00","user":{"id":5}}}],"total_comments":2,"access_control":{"can_comment":true,"is_locked":false},"next_url":"https://app-api.pixiv.net/v2/illust/comments?illust_id=123&offset=20"}`}
	result, err := comments.New(transport).List(context.Background(), comments.Request{ArtworkID: 123, Offset: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v2/illust/comments" || transport.query.Get("illust_id") != "123" || transport.query.Get("offset") != "10" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].Comment != "child" || result.Items[0].ParentComment == nil || result.Items[0].ParentComment.Comment != "parent" || result.Total == nil || *result.Total != 2 || result.AccessControl == nil || !result.HasNext || result.NextOffset != 20 {
		t.Fatalf("result = %#v", result)
	}
}

func TestCommentsRejectsInvalidParentID(t *testing.T) {
	_, err := comments.New(&fakeTransport{body: `{"comments":[{"id":1,"parent_comment":{"id":0}}]}`}).List(context.Background(), comments.Request{ArtworkID: 1})
	if err == nil {
		t.Fatal("invalid parent comment unexpectedly succeeded")
	}
}
