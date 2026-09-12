package novelbookmarks_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/novelbookmarks"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

type fakeTransport struct {
	path    string
	query   url.Values
	form    url.Values
	body    string
	getErr  error
	calls   int
	postErr error
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
	f.calls++
	f.path = path
	f.form = form
	return f.postErr
}

func TestListMapsBookmarkQueryAndContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[{"id":4,"title":"saved","user":{"id":5},"create_date":"2024-01-02T03:04:05+00:00"}],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?max_bookmark_id=8"}`}
	result, err := novelbookmarks.New(transport).List(context.Background(), novelbookmarks.Request{UserID: 7, Restrict: "private", Tag: "cat", MaxBookmarkID: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if transport.path != "/v1/user/bookmarks/novel" || transport.query.Get("user_id") != "7" || transport.query.Get("restrict") != "private" || transport.query.Get("tag") != "cat" || transport.query.Get("max_bookmark_id") != "3" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0].ID != 4 || result.NextMaxBookmarkID != 8 || !result.HasNext {
		t.Fatalf("result = %#v", result)
	}
}

func TestListRejectsInvalidBookmarkContinuation(t *testing.T) {
	transport := &fakeTransport{body: `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?max_bookmark_id=0"}`}
	if _, err := novelbookmarks.New(transport).List(context.Background(), novelbookmarks.Request{UserID: 7, Restrict: "public"}); err == nil {
		t.Fatal("invalid continuation unexpectedly succeeded")
	}
}

func TestNovelBookmarkTagsUseCandidatePathAndRequireBookmarkTags(t *testing.T) {
	transport := &fakeTransport{body: `{"bookmark_tags":[{"name":"cat","count":4}],"next_url":null}`}
	result, err := novelbookmarks.New(transport).Tags(context.Background(), novelbookmarks.TagsRequest{UserID: 7, Restrict: "private"})
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if transport.path != "/v1/user/bookmark-tags/novel" || len(transport.query) != 2 || transport.query.Get("user_id") != "7" || transport.query.Get("restrict") != "private" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if len(result.Items) != 1 || result.Items[0] != (novelbookmarks.BookmarkTag{Name: "cat", Count: 4}) {
		t.Fatalf("result = %#v", result)
	}
}

func TestNovelBookmarkDetailUsesCandidatePathAndPreservesState(t *testing.T) {
	transport := &fakeTransport{body: `{"bookmark_detail":{"is_bookmarked":true,"restrict":"private","tags":[{"name":"cat","is_registered":true}]}}`}
	result, err := novelbookmarks.New(transport).Detail(context.Background(), 42)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if transport.path != "/v2/novel/bookmark/detail" || len(transport.query) != 1 || transport.query.Get("novel_id") != "42" {
		t.Fatalf("request = %q %v", transport.path, transport.query)
	}
	if result.Restrict != "private" || len(result.Tags) != 1 || result.Tags[0] != "cat" {
		t.Fatalf("result = %#v", result)
	}
}

func TestNovelBookmarkMutationsUseCandidatePathsAndForms(t *testing.T) {
	transport := &fakeTransport{}
	client := novelbookmarks.New(transport)

	if err := client.Add(context.Background(), novelbookmarks.AddRequest{
		NovelID:  42,
		Restrict: "private",
		Tags:     []string{"cat", "favorite"},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if transport.path != "/v2/novel/bookmark/add" || len(transport.form) != 3 || transport.form.Get("novel_id") != "42" || transport.form.Get("restrict") != "private" || len(transport.form["tags[]"]) != 2 || transport.form["tags[]"][0] != "cat" || transport.form["tags[]"][1] != "favorite" {
		t.Fatalf("add request = %q %v", transport.path, transport.form)
	}

	if err := client.Remove(context.Background(), 42); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if transport.path != "/v1/novel/bookmark/delete" || len(transport.form) != 1 || transport.form.Get("novel_id") != "42" {
		t.Fatalf("remove request = %q %v", transport.path, transport.form)
	}
	if transport.calls != 2 {
		t.Fatalf("mutation transport calls = %d, want 2", transport.calls)
	}
}

func TestNovelBookmarkMutationsRejectInvalidRequestsBeforeTransport(t *testing.T) {
	tests := []struct {
		name string
		call func(*novelbookmarks.Client) error
	}{
		{name: "add zero novel", call: func(client *novelbookmarks.Client) error {
			return client.Add(context.Background(), novelbookmarks.AddRequest{NovelID: 0, Restrict: "public"})
		}},
		{name: "add negative novel", call: func(client *novelbookmarks.Client) error {
			return client.Add(context.Background(), novelbookmarks.AddRequest{NovelID: -1, Restrict: "public"})
		}},
		{name: "add empty restrict", call: func(client *novelbookmarks.Client) error {
			return client.Add(context.Background(), novelbookmarks.AddRequest{NovelID: 42})
		}},
		{name: "add unknown restrict", call: func(client *novelbookmarks.Client) error {
			return client.Add(context.Background(), novelbookmarks.AddRequest{NovelID: 42, Restrict: "friends"})
		}},
		{name: "remove zero novel", call: func(client *novelbookmarks.Client) error {
			return client.Remove(context.Background(), 0)
		}},
		{name: "remove negative novel", call: func(client *novelbookmarks.Client) error {
			return client.Remove(context.Background(), -1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &fakeTransport{}
			if err := test.call(novelbookmarks.New(transport)); err == nil {
				t.Fatal("invalid novel bookmark request unexpectedly succeeded")
			}
			if transport.calls != 0 {
				t.Fatalf("invalid request reached transport %d time(s)", transport.calls)
			}
		})
	}
}

func TestNovelBookmarkMutationsPropagateTransportErrors(t *testing.T) {
	wantErr := errors.New("novel bookmark mutation transport failed")
	addTransport := &fakeTransport{postErr: wantErr}
	if err := novelbookmarks.New(addTransport).Add(context.Background(), novelbookmarks.AddRequest{NovelID: 42, Restrict: "public"}); !errors.Is(err, wantErr) {
		t.Fatalf("Add error = %v, want %v", err, wantErr)
	}
	if addTransport.calls != 1 {
		t.Fatalf("Add transport calls = %d, want 1", addTransport.calls)
	}

	removeTransport := &fakeTransport{postErr: wantErr}
	if err := novelbookmarks.New(removeTransport).Remove(context.Background(), 42); !errors.Is(err, wantErr) {
		t.Fatalf("Remove error = %v, want %v", err, wantErr)
	}
	if removeTransport.calls != 1 {
		t.Fatalf("Remove transport calls = %d, want 1", removeTransport.calls)
	}
}

func TestNovelBookmarkListRejectsMalformedEnvelopeAndKeepsEmptyPage(t *testing.T) {
	for name, body := range map[string]string{
		"missing novels":      `{}`,
		"null novels":         `{"novels":null}`,
		"empty next url":      `{"novels":[],"next_url":""}`,
		"missing cursor":      `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?tag=cat"}`,
		"non-positive cursor": `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?max_bookmark_id=0"}`,
		"duplicate cursor":    `{"novels":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?max_bookmark_id=2&max_bookmark_id=3"}`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := novelbookmarks.New(&fakeTransport{body: body}).List(context.Background(), novelbookmarks.Request{UserID: 7, Restrict: "public"})
			if !errors.Is(err, protocol.ErrMalformedResponse) {
				t.Fatalf("List(%s) error = %v, want malformed response", body, err)
			}
			if result.Items != nil {
				t.Fatalf("malformed result contains partial items: %#v", result.Items)
			}
		})
	}

	result, err := novelbookmarks.New(&fakeTransport{body: `{"novels":[],"next_url":null}`}).List(context.Background(), novelbookmarks.Request{UserID: 7, Restrict: "public"})
	if err != nil {
		t.Fatalf("empty List: %v", err)
	}
	if result.Items == nil || len(result.Items) != 0 || result.HasNext {
		t.Fatalf("empty list result = %#v, want non-nil empty terminal page", result)
	}
}

func TestNovelBookmarkTagsRejectMalformedCandidatePayload(t *testing.T) {
	for name, body := range map[string]string{
		"missing tags":            `{}`,
		"null tags":               `{"bookmark_tags":null}`,
		"empty name":              `{"bookmark_tags":[{"name":"","count":1}]}`,
		"unverified continuation": `{"bookmark_tags":[],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/novel?offset=2"}`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := novelbookmarks.New(&fakeTransport{body: body}).Tags(context.Background(), novelbookmarks.TagsRequest{UserID: 7, Restrict: "public"})
			if !errors.Is(err, protocol.ErrMalformedResponse) {
				t.Fatalf("Tags(%s) error = %v, want malformed response", body, err)
			}
			if result.Items != nil {
				t.Fatalf("malformed result contains partial items: %#v", result.Items)
			}
		})
	}

	result, err := novelbookmarks.New(&fakeTransport{body: `{"bookmark_tags":[],"next_url":null}`}).Tags(context.Background(), novelbookmarks.TagsRequest{UserID: 7, Restrict: "public"})
	if err != nil {
		t.Fatalf("empty Tags: %v", err)
	}
	if result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("empty tags result = %#v, want non-nil empty tags", result)
	}
}

func TestNovelBookmarkDetailNormalizesCandidateAbsentStates(t *testing.T) {
	for name, body := range map[string]string{
		"missing detail": `{}`,
		"null detail":    `{"bookmark_detail":null}`,
		"false detail":   `{"bookmark_detail":{"is_bookmarked":false,"restrict":"","tags":[]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := novelbookmarks.New(&fakeTransport{body: body}).Detail(context.Background(), 42)
			if err != nil {
				t.Fatalf("Detail(%s): %v", body, err)
			}
			if result.Restrict != "" || result.Tags == nil || len(result.Tags) != 0 {
				t.Fatalf("absent detail = %#v", result)
			}
		})
	}

	result, err := novelbookmarks.New(&fakeTransport{getErr: protocol.HTTPStatus(http.StatusNotFound)}).Detail(context.Background(), 42)
	if err != nil {
		t.Fatalf("404 Detail: %v", err)
	}
	if result.Restrict != "" || result.Tags == nil || len(result.Tags) != 0 {
		t.Fatalf("404 absent detail = %#v", result)
	}

	for name, body := range map[string]string{
		"false detail with restrict":   `{"bookmark_detail":{"is_bookmarked":false,"restrict":"private","tags":[]}}`,
		"false detail with novel tags": `{"bookmark_detail":{"is_bookmarked":false,"restrict":"","tags":[{"name":"cat","is_registered":false}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			result, err := novelbookmarks.New(&fakeTransport{body: body}).Detail(context.Background(), 42)
			if err != nil {
				t.Fatalf("Detail(%s): %v", body, err)
			}
			if result.Restrict != "" || result.Tags == nil || len(result.Tags) != 0 {
				t.Fatalf("unbookmarked detail = %#v, want empty normalized state", result)
			}
		})
	}
}

func TestNovelBookmarkReadPropagatesTransportErrors(t *testing.T) {
	transportErr := errors.New("novel bookmark transport failed")
	for _, test := range []struct {
		name string
		call func(*fakeTransport) error
	}{
		{
			name: "list",
			call: func(transport *fakeTransport) error {
				_, err := novelbookmarks.New(transport).List(context.Background(), novelbookmarks.Request{UserID: 7, Restrict: "public"})
				return err
			},
		},
		{
			name: "tags",
			call: func(transport *fakeTransport) error {
				_, err := novelbookmarks.New(transport).Tags(context.Background(), novelbookmarks.TagsRequest{UserID: 7, Restrict: "public"})
				return err
			},
		},
		{
			name: "detail",
			call: func(transport *fakeTransport) error {
				_, err := novelbookmarks.New(transport).Detail(context.Background(), 42)
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
