package fanbox_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func homeMetadataBody(userID int64, name string) string {
	return `<html><head><meta name="metadata" content='{"context":{"user":{"userId":` + strconv.FormatInt(userID, 10) + `,"name":"` + name + `"}}}'></head></html>`
}

func testClient(t *testing.T, rt roundTripFunc) *fanbox.Client {
	t.Helper()
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, fanbox.Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return client
}

func TestValidateSession(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "www.fanbox.cc" {
			return nil, errors.New("unexpected host " + req.URL.Host)
		}
		if req.Header.Get("Cookie") == "" || !strings.Contains(req.Header.Get("Cookie"), "FANBOXSESSID") {
			t.Error("session cookie not sent to home host")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
	})
	client := testClient(t, rt)
	if err := client.ValidateSession(context.Background()); err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
}

func TestValidateSessionExpired(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	client := testClient(t, rt)
	if err := client.ValidateSession(context.Background()); sdk.CodeOf(err) != sdk.CodeCredentialsExpired {
		t.Fatalf("expected CodeCredentialsExpired, got %v", err)
	}
}

func TestCurrentUser(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
	})
	client := testClient(t, rt)
	user, err := client.CurrentUser(context.Background(), fanbox.CurrentUserRequest{})
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if user.UserID != 42 || user.DisplayName != "tester" {
		t.Fatalf("user = %+v", user)
	}
}

func TestPost(t *testing.T) {
	body := `{"body":{"post":{"id":"p1","title":"hello","publishedDatetime":"2024-06-01T10:00:00Z","creatorId":"pixiv","feeRequired":0,"isRestricted":false,"isPinned":false,"body":{"text":"caption","images":[{"id":"img1","extension":"png","originalUrl":"https://i.pximg.net/img/1.png","thumbnailUrl":"https://i.pximg.net/img/1_th.png"}],"files":[]}}}}`
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.fanbox.cc" || req.URL.Path != "/post.info" {
			t.Errorf("request = %s %s", req.Method, req.URL.String())
		}
		return jsonResponse(body), nil
	})
	client := testClient(t, rt)
	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p1"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if post.ID != "p1" || post.Title != "hello" || post.CreatorID != "pixiv" {
		t.Fatalf("post = %+v", post)
	}
	if post.Body == nil || len(post.Body.Assets) != 1 || post.Body.Assets[0].Resource.URL == "" {
		t.Fatalf("post body = %+v", post.Body)
	}
	if post.Body.Assets[0].Resource.RequestHeaders["Referer"] == "" {
		t.Fatal("image resource should carry a referer")
	}
}

func TestCreatorPostsPagination(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "api.fanbox.cc" || req.URL.Path != "/post.listCreator" {
			t.Errorf("request = %s", req.URL.String())
		}
		if calls == 1 {
			return jsonResponse(`{"body":{"posts":[{"id":"p1","title":"a","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"nextUrl":"https://api.fanbox.cc/post.listCreator?creatorId=c&maxPublishedDatetime=2024-01-02"}}`), nil
		}
		return jsonResponse(`{"body":{"posts":[{"id":"p2","title":"b","publishedDatetime":"2024-01-03T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}]}}`), nil
	})
	client := testClient(t, rt)
	page, err := client.CreatorPosts(context.Background(), fanbox.CreatorPostsRequest{CreatorID: "c"})
	if err != nil {
		t.Fatalf("CreatorPosts: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "p1" {
		t.Fatalf("page items = %+v", page.Items)
	}
	if page.Next.IsZero() {
		t.Fatal("expected continuation cursor")
	}
	page2, err := client.CreatorPosts(context.Background(), fanbox.CreatorPostsRequest{CreatorID: "c", Cursor: page.Next})
	if err != nil {
		t.Fatalf("continuation: %v", err)
	}
	if len(page2.Items) != 1 || page2.Items[0].ID != "p2" || calls != 2 {
		t.Fatalf("page2 = %+v calls=%d", page2.Items, calls)
	}
}

func TestSessionCredentialsRedaction(t *testing.T) {
	creds := fanbox.SessionCredentials{FANBOXSESSID: "super-secret-session"}
	for _, s := range []string{creds.String(), creds.GoString()} {
		if strings.Contains(s, "super-secret-session") {
			t.Fatalf("session leaked in %q", s)
		}
	}
}
