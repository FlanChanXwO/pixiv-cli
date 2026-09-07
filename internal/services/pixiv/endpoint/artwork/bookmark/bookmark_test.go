package bookmark_test

import (
	"context"
	"encoding/json"
	"errors"
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
	if _, ok := transport.query["type"]; ok {
		t.Fatalf("request unexpectedly includes unverified type filter: %v", transport.query)
	}
	if _, ok := transport.query["content_type"]; ok {
		t.Fatalf("request unexpectedly includes unverified content_type filter: %v", transport.query)
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
	if _, ok := transport.query["type"]; ok {
		t.Fatalf("tags request unexpectedly includes unverified type filter: %v", transport.query)
	}
	if _, ok := transport.query["content_type"]; ok {
		t.Fatalf("tags request unexpectedly includes unverified content_type filter: %v", transport.query)
	}

	transport.body = `{"bookmark_detail":{"is_bookmarked":true,"restrict":"private","tags":[{"name":"cat","is_registered":true}]}}`
	detail, err := bookmark.New(transport).Detail(context.Background(), 9)
	if err != nil || transport.path != "/v2/illust/bookmark/detail" || detail.Restrict != "private" || len(detail.Tags) != 1 || detail.Tags[0] != "cat" {
		t.Fatalf("detail = %#v err=%v", detail, err)
	}
	transport.body = `{"bookmark_detail":{"is_bookmarked":false,"restrict":"","tags":[]}}`
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

func TestBookmarkTagsRequireBookmarkTagsList(t *testing.T) {
	for _, body := range []string{`{}`, `{"bookmark_tags":null}`} {
		t.Run(body, func(t *testing.T) {
			_, err := bookmark.New(&fakeTransport{body: body}).Tags(context.Background(), bookmark.TagsRequest{UserID: 7, Restrict: "public"})
			if !errors.Is(err, protocol.ErrMalformedResponse) {
				t.Fatalf("Tags(%s) error = %v, want malformed response", body, err)
			}
		})
	}

	result, err := bookmark.New(&fakeTransport{body: `{"bookmark_tags":[],"next_url":null}`}).Tags(context.Background(), bookmark.TagsRequest{UserID: 7, Restrict: "public"})
	if err != nil {
		t.Fatalf("empty Tags: %v", err)
	}
	if result.Items == nil || len(result.Items) != 0 || result.HasNext {
		t.Fatalf("empty tags result = %#v, want non-nil empty terminal page", result)
	}
}

func TestBookmarkArtworksRejectMalformedEnvelopeAndKeepEmptyPage(t *testing.T) {
	invalidBodies := map[string]string{
		"missing illusts":      `{}`,
		"null illusts":         `{"illusts":null}`,
		"empty next url":       `{"illusts":[],"next_url":""}`,
		"missing continuation": `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?tag=cat"}`,
		"non-positive cursor":  `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?max_bookmark_id=0"}`,
		"duplicate cursor":     `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?max_bookmark_id=2&max_bookmark_id=3"}`,
	}
	for name, body := range invalidBodies {
		t.Run(name, func(t *testing.T) {
			result, err := bookmark.New(&fakeTransport{body: body}).Artworks(context.Background(), bookmark.ArtworksRequest{UserID: 7, Restrict: "public"})
			if !errors.Is(err, protocol.ErrMalformedResponse) {
				t.Fatalf("Artworks(%s) error = %v, want malformed response", body, err)
			}
			if result.Items != nil {
				t.Fatalf("malformed result contains partial items: %#v", result.Items)
			}
		})
	}

	result, err := bookmark.New(&fakeTransport{body: `{"illusts":[],"next_url":null}`}).Artworks(context.Background(), bookmark.ArtworksRequest{UserID: 7, Restrict: "public"})
	if err != nil {
		t.Fatalf("empty Artworks: %v", err)
	}
	if result.Items == nil || len(result.Items) != 0 || result.HasNext {
		t.Fatalf("empty artworks result = %#v, want non-nil empty terminal page", result)
	}
}

func TestBookmarkReadPropagatesTransportErrors(t *testing.T) {
	transportErr := errors.New("bookmark transport failed")
	for _, test := range []struct {
		name string
		call func(*fakeTransport) error
	}{
		{
			name: "artworks",
			call: func(transport *fakeTransport) error {
				_, err := bookmark.New(transport).Artworks(context.Background(), bookmark.ArtworksRequest{UserID: 7, Restrict: "public"})
				return err
			},
		},
		{
			name: "tags",
			call: func(transport *fakeTransport) error {
				_, err := bookmark.New(transport).Tags(context.Background(), bookmark.TagsRequest{UserID: 7, Restrict: "public"})
				return err
			},
		},
		{
			name: "detail",
			call: func(transport *fakeTransport) error {
				_, err := bookmark.New(transport).Detail(context.Background(), 9)
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(&fakeTransport{getErr: transportErr}); !errors.Is(err, transportErr) {
				t.Fatalf("error = %v, want transport error", err)
			}
		})
	}
}

func TestBookmarkTagsRejectMalformedItemsAndContinuation(t *testing.T) {
	invalidBodies := map[string]string{
		"empty name":           `{"bookmark_tags":[{"name":"","count":1}]}`,
		"empty next url":       `{"bookmark_tags":[],"next_url":""}`,
		"missing continuation": `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?tag=cat"}`,
		"non-positive offset":  `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?offset=0"}`,
		"duplicate offset":     `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?offset=2&offset=3"}`,
	}
	for name, body := range invalidBodies {
		t.Run(name, func(t *testing.T) {
			result, err := bookmark.New(&fakeTransport{body: body}).Tags(context.Background(), bookmark.TagsRequest{UserID: 7, Restrict: "public"})
			if !errors.Is(err, protocol.ErrMalformedResponse) {
				t.Fatalf("Tags(%s) error = %v, want malformed response", body, err)
			}
			if result.Items != nil {
				t.Fatalf("malformed result contains partial items: %#v", result.Items)
			}
		})
	}
}

func TestBookmarkDetailRejectsContradictoryUnbookmarkedFields(t *testing.T) {
	for _, body := range []string{
		`{"bookmark_detail":{"is_bookmarked":false,"restrict":"private","tags":[]}}`,
		`{"bookmark_detail":{"is_bookmarked":false,"restrict":"","tags":[{"name":"cat"}]}}`,
	} {
		t.Run(body, func(t *testing.T) {
			result, err := bookmark.New(&fakeTransport{body: body}).Detail(context.Background(), 9)
			if !errors.Is(err, protocol.ErrMalformedResponse) {
				t.Fatalf("Detail(%s) error = %v, want malformed response", body, err)
			}
			if result.Tags != nil || result.Restrict != "" {
				t.Fatalf("malformed detail result = %#v", result)
			}
		})
	}
}
