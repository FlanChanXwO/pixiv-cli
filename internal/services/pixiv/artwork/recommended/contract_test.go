package recommended_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork/recommended"
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

func TestRecommendedPreservesInitialOffsetPolicyAndMapsArtwork(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[{"id":9,"title":"recommended","user":{"id":10},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/illust/recommended?offset=0"}`}
	result, err := recommended.New(transport).List(context.Background(), recommended.Request{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v1/illust/recommended" {
		t.Fatalf("path = %q", transport.path)
	}
	if _, present := transport.query["offset"]; present {
		t.Fatalf("initial query unexpectedly contains offset: %v", transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 9 || result.NextOffset != 0 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestRecommendedSendsZeroOffsetForContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[]}`}
	_, err := recommended.New(transport).List(context.Background(), recommended.Request{Offset: 0, ContinuationExists: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.query.Get("offset") != "0" {
		t.Fatalf("query = %v", transport.query)
	}
}
