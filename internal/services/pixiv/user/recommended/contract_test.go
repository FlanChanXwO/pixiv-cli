package recommended_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/user/recommended"
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

func TestRecommendedMapsNestedArtworkNovelAndContinuation(t *testing.T) {
	f := &fakeTransport{body: `{"user_previews":[{"user":{"id":111,"name":"artist"},"illusts":[{"id":112,"title":"art","user":{"id":111},"create_date":"2024-01-02T03:04:05+00:00"}],"novels":[{"id":113,"title":"story","user":{"id":111},"create_date":"2024-01-02T03:04:05+00:00"}]}],"next_url":"https://app-api.pixiv.net/v1/user/recommended?offset=0"}`}
	result, err := recommended.New(f).List(context.Background(), recommended.Request{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if f.path != "/v1/user/recommended" || f.query.Get("offset") != "" || len(result.Items) != 1 || result.Items[0].User.ID != 111 || len(result.Items[0].Illusts) != 1 || result.Items[0].Illusts[0].ID != 112 || len(result.Items[0].Novels) != 1 || result.Items[0].Novels[0].ID != 113 || result.NextOffset != 0 || !result.HasNext {
		t.Fatalf("result=%#v request=%q %v", result, f.path, f.query)
	}
}

func TestRecommendedRejectsInvalidNestedIDs(t *testing.T) {
	_, err := recommended.New(&fakeTransport{body: `{"user_previews":[{"user":{"id":111},"illusts":[{"id":0}],"novels":[]}]}`}).List(context.Background(), recommended.Request{})
	if err == nil {
		t.Fatal("invalid nested artwork unexpectedly succeeded")
	}
}
