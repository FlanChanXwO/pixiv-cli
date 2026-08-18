package fanbox_test

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"

	"github.com/stretchr/testify/require"
	"io"
	"net/http"

	"strconv"
	"strings"
	"sync"
	"testing"
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
	if err := client.ValidateSession(context.Background()); sdk.ReasonOf(err) != sdk.CredentialsExpired {
		t.Fatalf("expected CredentialsExpired, got %v", err)
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

func TestCreatorsPagination(t *testing.T) {
	const nextURL = "https://api.fanbox.cc/plan.listSupporting?page=2"
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "api.fanbox.cc" || req.URL.Path != "/plan.listSupporting" {
			t.Errorf("request = %s", req.URL.String())
		}
		if calls == 1 {
			return jsonResponse(`{"body":{"plans":[{"creatorId":"supported-1"}],"pageUrls":["` + nextURL + `"]}}`), nil
		}
		if req.URL.String() != nextURL {
			t.Errorf("continuation request = %s, want %s", req.URL.String(), nextURL)
		}
		return jsonResponse(`{"body":{"plans":[{"creatorId":"supported-2"}]}}`), nil
	})
	client := testClient(t, rt)

	page, err := client.Creators(context.Background(), fanbox.CreatorsRequest{Kind: fanbox.CreatorListSupporting})
	if err != nil {
		t.Fatalf("Creators page 1: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "supported-1" {
		t.Fatalf("page 1 = %+v", page.Items)
	}
	if page.Next.IsZero() {
		t.Fatal("Creators did not return a continuation cursor")
	}

	page2, err := client.Creators(context.Background(), fanbox.CreatorsRequest{
		Kind:   fanbox.CreatorListSupporting,
		Cursor: page.Next,
	})
	if err != nil {
		t.Fatalf("Creators page 2: %v", err)
	}
	if len(page2.Items) != 1 || page2.Items[0].ID != "supported-2" || calls != 2 {
		t.Fatalf("page 2 = %+v calls=%d", page2.Items, calls)
	}
}

func TestOpenWithUsesCustomUserAgentOnlyForNativeRequests(t *testing.T) {
	const wantUA = "fanbox-test-agent/1"
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("User-Agent"); got != wantUA {
			t.Errorf("User-Agent = %q, want %q", got, wantUA)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(homeMetadataBody(42, "tester")))}, nil
	})
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, fanbox.Options{
		HTTPClient: &http.Client{Transport: rt},
		UserAgent:  wantUA,
	})
	require.NoError(t, err)
	require.NoError(t, client.ValidateSession(context.Background()))
}

func TestOpenWithRejectsInvalidNativeUserAgentBeforeTransport(t *testing.T) {
	called := false
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, fanbox.Options{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("unexpected transport call")
		})},
		UserAgent: "invalid\nagent",
	})
	require.Error(t, err)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	require.Nil(t, client)
	require.False(t, called)
}

func TestOpenWithValidatesIndependentFlareSolverrAddresses(t *testing.T) {
	for _, test := range []struct {
		name    string
		options fanbox.Options
	}{
		{name: "service userinfo", options: fanbox.Options{FlareSolverr: &fanbox.FlareSolverrOptions{URL: "http://user:pass@solver.example"}}},
		{name: "service path prefix", options: fanbox.Options{FlareSolverr: &fanbox.FlareSolverrOptions{URL: "http://solver.example/prefix"}}},
		{name: "upstream userinfo", options: fanbox.Options{FlareSolverr: &fanbox.FlareSolverrOptions{URL: "http://solver.example", ProxyURL: "socks5://user:pass@proxy.example"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "session-value"}, test.options)
			require.Error(t, err)
			require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
			require.NotContains(t, err.Error(), "user")
			require.NotContains(t, err.Error(), "pass")
		})
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

func TestPostMapsFileMapWhenFilesListIsOmitted(t *testing.T) {
	body := `{"body":{"post":{"id":"p-file-map","title":"file map","publishedDatetime":"2024-06-01T10:00:00Z","creatorId":"pixiv","feeRequired":0,"isRestricted":false,"isPinned":false,"body":{"blocks":[{"type":"file","fileId":"file-1"}],"fileMap":{"file-1":{"id":"file-1","name":"clip.mp4","extension":"mp4","url":"https://downloads.fanbox.cc/file-1.mp4"}}}}}}`
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(body), nil
	})
	client := testClient(t, rt)

	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p-file-map"})
	require.NoError(t, err)
	require.NotNil(t, post.Body)
	require.Len(t, post.Body.Assets, 1)
	require.Equal(t, fanbox.AssetKindFile, post.Body.Assets[0].Kind)
	require.Equal(t, "clip.mp4", post.Body.Assets[0].Name)
	require.Equal(t, "https://downloads.fanbox.cc/file-1.mp4", post.Body.Assets[0].Resource.URL)
	require.Len(t, post.Body.Blocks, 1)
	require.Equal(t, fanbox.PostBlockFile, post.Body.Blocks[0].Kind)
	require.NotNil(t, post.Body.Blocks[0].File)
	require.Equal(t, "clip.mp4", post.Body.Blocks[0].File.Name)
}

func TestOpenResourceUsesSessionOnlyForDownloadsHost(t *testing.T) {
	var mediaCookie string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info":
			return jsonResponse(`{"body":{"post":{"id":"p-resource","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"https://downloads.fanbox.cc/image-1.png"}]}}}}`), nil
		case req.URL.Host == "downloads.fanbox.cc":
			mediaCookie = req.Header.Get("Cookie")
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("bytes"))}, nil
		default:
			return nil, errors.New("unexpected FANBOX resource request")
		}
	})
	client := testClient(t, rt)
	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p-resource"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if len(post.Body.Assets) != 1 || post.Body.Assets[0].Resource.Ref.IsZero() {
		t.Fatalf("post resource = %+v", post.Body)
	}
	response, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: post.Body.Assets[0].Resource.Ref, Method: sdk.ResourceMethodGet})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer response.Body.Close()
	if mediaCookie != "FANBOXSESSID=session-value" {
		t.Fatalf("downloads Cookie = %q", mediaCookie)
	}
}

func TestOpenResourceDoesNotSendSessionToPixivMediaHost(t *testing.T) {
	var mediaCookie string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info":
			return jsonResponse(`{"body":{"post":{"id":"p-pixiv-resource","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"https://i.pximg.net/image-1.png"}]}}}}`), nil
		case req.URL.Host == "i.pximg.net":
			mediaCookie = req.Header.Get("Cookie")
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("bytes"))}, nil
		default:
			return nil, errors.New("unexpected FANBOX resource request")
		}
	})
	client := testClient(t, rt)
	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p-pixiv-resource"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	response, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: post.Body.Assets[0].Resource.Ref, Method: sdk.ResourceMethodGet})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer response.Body.Close()
	if mediaCookie != "" {
		t.Fatalf("Pixiv media Cookie = %q", mediaCookie)
	}
}

func TestOpenResourceForwardsMethodAndConditionalHeaders(t *testing.T) {
	var mediaRequest *http.Request
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "api.fanbox.cc" && req.URL.Path == "/post.info":
			return jsonResponse(`{"body":{"post":{"id":"p-conditional","title":"resource","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false,"body":{"images":[{"id":"image-1","originalUrl":"https://downloads.fanbox.cc/image-1.png"}]}}}}`), nil
		case req.URL.Host == "downloads.fanbox.cc":
			copy := req.Clone(req.Context())
			mediaRequest = copy
			return &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Length": {"2"}}, Body: io.NopCloser(strings.NewReader("ok"))}, nil
		default:
			return nil, errors.New("unexpected FANBOX resource request")
		}
	})
	client := testClient(t, rt)
	post, err := client.Post(context.Background(), fanbox.PostRequest{PostID: "p-conditional"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	response, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{
		Ref:             post.Body.Assets[0].Resource.Ref,
		Method:          sdk.ResourceMethodHead,
		Range:           "bytes=0-1",
		IfNoneMatch:     `"etag-1"`,
		IfModifiedSince: "Wed, 21 Oct 2015 07:28:00 GMT",
		IfRange:         `"etag-1"`,
	})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer response.Body.Close()
	if mediaRequest == nil {
		t.Fatal("resource transport was not called")
	}
	if mediaRequest.Method != http.MethodHead {
		t.Fatalf("resource method = %q, want HEAD", mediaRequest.Method)
	}
	for name, want := range map[string]string{
		"Range":             "bytes=0-1",
		"If-None-Match":     `"etag-1"`,
		"If-Modified-Since": "Wed, 21 Oct 2015 07:28:00 GMT",
		"If-Range":          `"etag-1"`,
	} {
		if got := mediaRequest.Header.Get(name); got != want {
			t.Errorf("resource header %s = %q, want %q", name, got, want)
		}
	}
}

func TestCreatorPostsPagination(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "api.fanbox.cc" || (req.URL.Path != "/post.listCreator" && req.URL.Path != "/post.paginateCreator") {
			t.Errorf("request = %s", req.URL.String())
		}
		if calls == 1 {
			if got := req.URL.Query().Get("limit"); got != "10" {
				t.Errorf("creator limit = %q, want 10", got)
			}
			return jsonResponse(`{"body":{"posts":[{"id":"p1","title":"a","publishedDatetime":"2024-01-01T00:00:00Z","creatorId":"c","isRestricted":false,"isPinned":false}],"pageUrls":["https://api.fanbox.cc/post.paginateCreator?creatorId=c&page=2"]}}`), nil
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
func TestParallelClientsKeepSessionCookiesAndTransportStateIsolated(t *testing.T) {
	type observation struct {
		mu     sync.Mutex
		cookie string
	}
	makeClient := func(session, postID string, observed *observation) *fanbox.Client {
		rt := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			observed.mu.Lock()
			observed.cookie = request.Header.Get("Cookie")
			observed.mu.Unlock()
			body := `{"body":{"post":{"id":"` + postID + `","title":"title","publishedDatetime":"2024-01-01T00:00:00Z","isRestricted":false,"isPinned":false}}}`
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: session}, fanbox.Options{HTTPClient: &http.Client{Transport: rt}})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	firstObservation := &observation{}
	secondObservation := &observation{}
	first := makeClient("first-session", "first-post", firstObservation)
	second := makeClient("second-session", "second-post", secondObservation)

	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		if _, err := first.Post(context.Background(), fanbox.PostRequest{PostID: "first-post"}); err != nil {
			t.Errorf("first client error = %v", err)
		}
	}()
	go func() {
		defer group.Done()
		if _, err := second.Post(context.Background(), fanbox.PostRequest{PostID: "second-post"}); err != nil {
			t.Errorf("second client error = %v", err)
		}
	}()
	group.Wait()

	if firstObservation.cookie != "FANBOXSESSID=first-session" {
		t.Fatalf("first cookie = %q", firstObservation.cookie)
	}
	if secondObservation.cookie != "FANBOXSESSID=second-session" {
		t.Fatalf("second cookie = %q", secondObservation.cookie)
	}
}
