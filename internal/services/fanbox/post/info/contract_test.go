package info_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/post/info"
)

type jsonTransport struct{ body string }

func (t *jsonTransport) GetJSON(_ context.Context, _ string, target any) error {
	return json.Unmarshal([]byte(t.body), target)
}

func TestGetRequiresPostInfoEnvelopeAndMapsBlocks(t *testing.T) {
	transport := &jsonTransport{body: `{"body":{"post":{"id":"post-1","title":"title","publishedDatetime":"2024-06-01T10:00:00Z","body":{"blocks":[{"type":"image","imageId":"image-1"}],"imageMap":{"image-1":{"id":"image-1","extension":"png","originalUrl":"https://downloads.fanbox.cc/image.png"}}}}}}`}
	value, err := info.New(transport).Get(context.Background(), info.Request{PostID: "post-1"})
	if err != nil {
		t.Fatal(err)
	}
	if value.ID != "post-1" || value.Body == nil || len(value.Body.Assets) != 1 || value.Body.Assets[0].ID != "image-1" {
		t.Fatalf("post = %+v", value)
	}
}

func TestGetRejectsMissingMediaReference(t *testing.T) {
	transport := &jsonTransport{body: `{"body":{"post":{"id":"post-bad","body":{"blocks":[{"type":"image","imageId":"missing"}],"imageMap":{}}}}}`}
	if _, err := info.New(transport).Get(context.Background(), info.Request{PostID: "post-bad"}); err == nil {
		t.Fatal("missing media reference unexpectedly succeeded")
	}
}
