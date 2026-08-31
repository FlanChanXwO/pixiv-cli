package bookmark_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/bookmark"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type fakeTransport struct {
	path   string
	query  url.Values
	form   url.Values
	body   string
	getErr error
}

func (f *fakeTransport) GetJSON(_ context.Context, path string, query url.Values, out any) error {
	f.path = path
	f.query = query
	if f.getErr != nil {
		return f.getErr
	}
	return json.Unmarshal([]byte(f.body), out)
}

func (f *fakeTransport) PostForm(_ context.Context, path string, form url.Values) error {
	f.path = path
	f.form = form
	return nil
}

func TestBookmarkArtworkListMapsQueryAndBookmarkContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"illusts":[{"id":4,"title":"saved","user":{"id":5},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?max_bookmark_id=8"}`}
	result, err := bookmark.New(transport).Artworks(context.Background(), bookmark.ArtworksRequest{UserID: 7, Restrict: "private", Tag: "cat", MaxBookmarkID: 3})
	if err != nil {
		t.Fatalf("Artworks: %v", err)
	}
	if transport.path != "/v1/user/bookmarks/illust" || transport.query.Get("user_id") != "7" || transport.query.Get("restrict") != "private" || transport.query.Get("tag") != "cat" || transport.query.Get("max_bookmark_id") != "3" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 4 || result.NextMaxBookmarkID != 8 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestBookmarkTagsDetailAndMutations(t *testing.T) {
	transport := &fakeTransport{body: `{"bookmark_tags":[{"name":"cat","count":2}],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?offset=5"}`}
	tags, err := bookmark.New(transport).Tags(context.Background(), bookmark.TagsRequest{UserID: 7, Restrict: "public", Offset: 2})
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if transport.path != "/v1/user/bookmark-tags/illust" || transport.query.Get("offset") != "2" || len(tags.Items) != 1 || tags.NextOffset != 5 {
		t.Fatalf("tags = %#v request=%q %v", tags, transport.path, transport.query)
	}

	transport.body = `{"bookmark_detail":{"is_bookmarked":true,"restrict":"private","tags":[{"name":"cat","is_registered":true}]}}`
	detail, err := bookmark.New(transport).Detail(context.Background(), 9)
	if err != nil || transport.path != "/v2/illust/bookmark/detail" || detail.Restrict != "private" || len(detail.Tags) != 1 || detail.Tags[0] != "cat" {
		t.Fatalf("detail = %#v err=%v", detail, err)
	}
	transport.body = `{"bookmark_detail":{"is_bookmarked":false,"restrict":"public","tags":[{"name":"cat","is_registered":false}]}}`
	detail, err = bookmark.New(transport).Detail(context.Background(), 9)
	if err != nil || detail.Restrict != "" || detail.Tags == nil || len(detail.Tags) != 0 {
		t.Fatalf("not bookmarked detail = %#v err=%v", detail, err)
	}
	transport.body = `{"bookmark_detail":null}`
	detail, err = bookmark.New(transport).Detail(context.Background(), 9)
	if err != nil || detail.Restrict != "" || detail.Tags == nil || len(detail.Tags) != 0 {
		t.Fatalf("null bookmark detail = %#v err=%v", detail, err)
	}
	transport.getErr = protocol.HTTPStatus(http.StatusNotFound)
	detail, err = bookmark.New(transport).Detail(context.Background(), 9)
	if err != nil || detail.Restrict != "" || detail.Tags == nil || len(detail.Tags) != 0 {
		t.Fatalf("404 bookmark detail = %#v err=%v", detail, err)
	}
	transport.getErr = nil

	if err := bookmark.New(transport).Add(context.Background(), bookmark.AddRequest{ArtworkID: 9, Restrict: "public", Tags: []string{"cat", "favorite"}}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if transport.path != "/v2/illust/bookmark/add" || transport.form.Get("illust_id") != "9" || len(transport.form["tags[]"]) != 2 {
		t.Fatalf("add request = %q %v", transport.path, transport.form)
	}
	if err := bookmark.New(transport).Remove(context.Background(), 9); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if transport.path != "/v1/illust/bookmark/delete" || transport.form.Get("illust_id") != "9" {
		t.Fatalf("remove request = %q %v", transport.path, transport.form)
	}
}
