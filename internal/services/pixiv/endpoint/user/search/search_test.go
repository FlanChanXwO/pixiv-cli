package search_test

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/search"
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

func TestSearchMapsRouteQueryUsersAndContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"user_previews":[{"user":{"id":41,"name":"artist","account":"artist"}}],"next_url":"https://app-api.pixiv.net/v1/search/user?word=artist&offset=20"}`}
	result, err := search.New(transport).Search(context.Background(), search.Request{Word: "artist", Offset: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if transport.path != "/v1/search/user" || transport.query.Get("word") != "artist" || transport.query.Get("offset") != "5" || len(result.Items) != 1 || result.Items[0].User.ID != 41 || result.NextOffset != 20 || !result.HasNext {
		t.Fatalf("result=%#v request=%q %v", result, transport.path, transport.query)
	}
}

func TestSearchRejectsNullUserList(t *testing.T) {
	_, err := search.New(&fakeTransport{body: `{"user_previews":null}`}).Search(context.Background(), search.Request{Word: "artist"})
	if err == nil {
		t.Fatal("null user list unexpectedly succeeded")
	}
}
