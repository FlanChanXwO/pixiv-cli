package pixiv_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestBookmarkOperationsRejectUnknownRestrictBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, io.ErrUnexpectedEOF
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	tests := []struct {
		name string
		call func() error
	}{
		{name: "artwork bookmarks", call: func() error {
			_, err := client.UserArtworkBookmarks(context.Background(), UserArtworkBookmarksRequest{UserID: 7, Restrict: Restrict("unknown")})
			return err
		}},
		{name: "bookmark tags", call: func() error {
			_, err := client.UserArtworkBookmarkTags(context.Background(), UserArtworkBookmarkTagsRequest{UserID: 7, Restrict: Restrict("unknown")})
			return err
		}},
		{name: "novel bookmarks", call: func() error {
			_, err := client.UserNovelBookmarks(context.Background(), UserNovelBookmarksRequest{UserID: 7, Restrict: Restrict("unknown")})
			return err
		}},
		{name: "add bookmark", call: func() error {
			return client.AddBookmark(context.Background(), AddBookmarkRequest{ArtworkID: 7, Restrict: Restrict("unknown")})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid restrict reached upstream %d time(s)", calls)
	}
}

func TestSearchArtworksWiresBookmarkCandidateBounds(t *testing.T) {
	minimum, maximum := 10, 100
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		query := req.URL.Query()
		if query.Get("bookmark_num_min") != "10" || query.Get("bookmark_num_max") != "100" {
			t.Errorf("bookmark candidate query = %v", query)
		}
		body := `{"illusts":[],"next_url":null}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	page, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{
		Word: "bookmarks", BookmarkMin: &minimum, BookmarkMax: &maximum,
	})
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if page.Items == nil {
		t.Fatal("successful empty page must contain a non-nil Items slice")
	}
}

func TestUserArtworkBookmarksWiresQueryCursorAndResource(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/user/bookmarks/illust" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("user_id") != "7" || query.Get("restrict") != "private" || query.Get("tag") != "cat" {
			t.Errorf("query = %v", query)
		}
		wantMax := ""
		if calls == 2 {
			wantMax = "88"
		}
		if query.Get("max_bookmark_id") != wantMax {
			t.Errorf("max_bookmark_id = %q, want %q", query.Get("max_bookmark_id"), wantMax)
		}
		body := `{"illusts":[{"id":1001,"title":"bookmarked","type":"illust","create_date":"2026-01-01T00:00:00Z","total_bookmarks":42,"image_urls":{"original":"https://i.pximg.net/img/1001.png"},"user":{"id":7,"name":"artist"},"tags":[]}],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/illust?user_id=7&restrict=private&tag=cat&max_bookmark_id=88"}`
		if calls == 2 {
			body = `{"illusts":[],"next_url":null}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := UserArtworkBookmarksRequest{UserID: 7, Restrict: RestrictPrivate, Tag: "cat"}
	page, err := client.UserArtworkBookmarks(context.Background(), request)
	if err != nil {
		t.Fatalf("UserArtworkBookmarks: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 1001 || page.Items[0].TotalBookmarks != 42 {
		t.Fatalf("page = %#v", page)
	}
	if page.Items[0].Cover.Resource.Ref.IsZero() || page.Items[0].Cover.Resource.URL == "" {
		t.Fatal("bookmark artwork did not carry a usable cover resource")
	}
	if page.Next.IsZero() {
		t.Fatal("expected bookmark continuation")
	}
	request.Cursor = page.Next
	page, err = client.UserArtworkBookmarks(context.Background(), request)
	if err != nil {
		t.Fatalf("UserArtworkBookmarks continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("continuation page = %#v calls=%d", page, calls)
	}
}

func TestUserArtworkBookmarkTagsWiresQueryCursorAndEmptyPage(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/user/bookmark-tags/illust" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("user_id") != "7" || query.Get("restrict") != "public" {
			t.Errorf("query = %v", query)
		}
		wantOffset := ""
		if calls == 2 {
			wantOffset = "4"
		}
		if query.Get("offset") != wantOffset {
			t.Errorf("offset = %q, want %q", query.Get("offset"), wantOffset)
		}
		body := `{"bookmark_tags":[{"name":"cat","count":3}],"next_url":"https://app-api.pixiv.net/v1/user/bookmark-tags/illust?user_id=7&restrict=public&offset=4"}`
		if calls == 2 {
			body = `{"bookmark_tags":[],"next_url":null}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := UserArtworkBookmarkTagsRequest{UserID: 7, Restrict: RestrictPublic}
	page, err := client.UserArtworkBookmarkTags(context.Background(), request)
	if err != nil {
		t.Fatalf("UserArtworkBookmarkTags: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0] != (BookmarkTag{Name: "cat", Count: 3}) || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.UserArtworkBookmarkTags(context.Background(), request)
	if err != nil {
		t.Fatalf("UserArtworkBookmarkTags continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestUserNovelBookmarksWiresQueryCursorAndResource(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/user/bookmarks/novel" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("user_id") != "8" || query.Get("restrict") != "public" || query.Get("tag") != "story" {
			t.Errorf("query = %v", query)
		}
		wantMax := ""
		if calls == 2 {
			wantMax = "99"
		}
		if query.Get("max_bookmark_id") != wantMax {
			t.Errorf("max_bookmark_id = %q, want %q", query.Get("max_bookmark_id"), wantMax)
		}
		body := `{"novels":[{"id":2001,"title":"story","create_date":"2026-01-01T00:00:00Z","total_bookmarks":9,"image_urls":{"original":"https://i.pximg.net/img/novel-2001.png"},"user":{"id":8,"name":"writer"}}],"next_url":"https://app-api.pixiv.net/v1/user/bookmarks/novel?user_id=8&restrict=public&tag=story&max_bookmark_id=99"}`
		if calls == 2 {
			body = `{"novels":[],"next_url":null}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := UserNovelBookmarksRequest{UserID: 8, Restrict: RestrictPublic, Tag: "story"}
	page, err := client.UserNovelBookmarks(context.Background(), request)
	if err != nil {
		t.Fatalf("UserNovelBookmarks: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 2001 || page.Items[0].TotalBookmarks != 9 || page.Items[0].Cover.Resource.Ref.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.UserNovelBookmarks(context.Background(), request)
	if err != nil {
		t.Fatalf("UserNovelBookmarks continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestArtworkBookmarkPreservesBookmarkedAndAbsentStates(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/illust/bookmark/detail" || req.URL.Query().Get("illust_id") != "77" {
			t.Errorf("request = %s?%s", req.URL.Path, req.URL.RawQuery)
		}
		body := `{"bookmark_detail":{"restrict":"private","tags":["cat","fav"]}}`
		if calls == 2 {
			body = `{"bookmark_detail":null}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	detail, err := client.ArtworkBookmark(context.Background(), ArtworkBookmarkRequest{ArtworkID: 77})
	if err != nil {
		t.Fatalf("ArtworkBookmark: %v", err)
	}
	if detail.Restrict != RestrictPrivate || len(detail.Tags) != 2 {
		t.Fatalf("bookmarked detail = %#v", detail)
	}
	detail, err = client.ArtworkBookmark(context.Background(), ArtworkBookmarkRequest{ArtworkID: 77})
	if err != nil {
		t.Fatalf("ArtworkBookmark absent: %v", err)
	}
	if detail.Restrict != "" || detail.Tags == nil || len(detail.Tags) != 0 {
		t.Fatalf("absent bookmark state = %#v", detail)
	}
}

func TestResourceRefDoesNotExposeResolvedURL(t *testing.T) {
	const rawURL = "https://i.pximg.net/img/example.png?signature=sentinel"
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := `{"illust":{"id":42,"title":"art","type":"illust","create_date":"2026-01-01T00:00:00Z","image_urls":{"original":"` + rawURL + `"},"user":{"id":7,"name":"artist"},"tags":[]}}`
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	artwork, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 42})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	payload, err := sdk.ResourceRefPayload(artwork.Cover.Resource.Ref)
	if err != nil {
		t.Fatalf("ResourceRefPayload: %v", err)
	}
	if strings.Contains(string(payload), rawURL) || strings.Contains(string(payload), "signature") {
		t.Fatalf("resource reference payload exposes resolved URL: %q", payload)
	}
}

func TestOpenResourceRejectsForeignProductBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("foreign resource reached upstream")
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	ref, err := sdk.NewResourceRef("other-product", []byte(`{"k":"artwork","id":42}`))
	if err != nil {
		t.Fatalf("NewResourceRef: %v", err)
	}
	_, err = client.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: ref})
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if calls != 0 {
		t.Fatalf("foreign resource reached upstream %d time(s)", calls)
	}
}

func TestOpenResourceResolvesIdentityWithoutEmbeddedURL(t *testing.T) {
	const mediaURL = "https://i.pximg.net/img/42.png?signature=sentinel"
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "app-api.pixiv.net":
			if req.URL.Path != "/v1/illust/detail" {
				return nil, errors.New("unexpected app path")
			}
			body := `{"illust":{"id":42,"title":"resolved","type":"illust","create_date":"2026-01-01T00:00:00Z","image_urls":{"original":"` + mediaURL + `"},"user":{"id":7,"name":"artist"},"tags":[]}}`
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		case "i.pximg.net":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("DATA"))}, nil
		default:
			return nil, errors.New("unexpected resource host")
		}
	})
	httpClient := &http.Client{Transport: rt}
	first, err := NewWith("token", Options{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("NewWith first: %v", err)
	}
	artwork, err := first.Artwork(context.Background(), ArtworkRequest{ArtworkID: 42})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	second, err := NewWith("token", Options{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("NewWith second: %v", err)
	}
	response, err := second.OpenResource(context.Background(), sdk.OpenResourceRequest{Ref: artwork.Cover.Resource.Ref})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "DATA" {
		t.Fatalf("body = %q err=%v", body, err)
	}
}

func TestOpenResourceForwardsConditionalHeadersWithoutCookies(t *testing.T) {
	var received *http.Request
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received = req.Clone(req.Context())
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Range", "bytes 0-3/4")
		w.Header().Set("ETag", `"etag"`)
		w.Header().Set("Set-Cookie", "sentinel=must-not-leak")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "DATA")
	}))
	defer server.Close()
	client, httpClient := resourceTestClient(t, server)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	jar.SetCookies(parsed, []*http.Cookie{{Name: "sentinel", Value: "must-not-send"}})
	httpClient.Jar = jar
	artwork, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 1})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	response, err := client.OpenResource(context.Background(), sdk.OpenResourceRequest{
		Ref: artwork.Cover.Resource.Ref, Range: "bytes=0-3", IfNoneMatch: `"old"`, IfModifiedSince: "Wed, 01 Jan 2025 00:00:00 GMT", IfRange: `"old"`,
	})
	if err != nil {
		t.Fatalf("OpenResource: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "DATA" {
		t.Fatalf("body = %q err=%v", body, err)
	}
	if received == nil || received.Method != http.MethodGet || received.Header.Get("Range") != "bytes=0-3" || received.Header.Get("If-None-Match") != `"old"` || received.Header.Get("If-Modified-Since") == "" || received.Header.Get("If-Range") != `"old"` {
		t.Fatalf("forwarded request = %#v", received)
	}
	if received.Header.Get("Cookie") != "" {
		t.Fatalf("resource request carried a cookie: %q", received.Header.Get("Cookie"))
	}
	if response.Header().Get("Set-Cookie") != "" || response.ContentRange() != "bytes 0-3/4" || response.ETag() != `"etag"` {
		t.Fatalf("response headers = %#v", response.Header())
	}
}

func TestSaveResourceReportsProgressTotal(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4")
		_, _ = io.WriteString(w, "DATA")
	}))
	defer server.Close()
	client, _ := resourceTestClient(t, server)
	artwork, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 1})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	var progress []sdk.SaveProgress
	_, err = client.SaveResource(context.Background(), artwork.Cover.Resource.Ref, sdk.SaveOptions{
		Path: t.TempDir() + "/asset.bin",
		Progress: func(value sdk.SaveProgress) {
			progress = append(progress, value)
		},
	})
	if err != nil {
		t.Fatalf("SaveResource: %v", err)
	}
	if len(progress) == 0 || progress[len(progress)-1].Done != 4 || progress[len(progress)-1].Total != 4 {
		t.Fatalf("progress = %#v", progress)
	}
}

func resourceTestClient(t *testing.T, server *httptest.Server) (*Client, *http.Client) {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	serverClient := server.Client()
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "app-api.pixiv.net" {
			body := `{"illust":{"id":1,"title":"resource","type":"illust","create_date":"2026-01-01T00:00:00Z","image_urls":{"original":"` + server.URL + `/asset"},"user":{"id":7,"name":"artist"},"tags":[]}}`
			return jsonResponse(body), nil
		}
		return serverClient.Transport.RoundTrip(request)
	})}
	client, err := NewWith("token", Options{HTTPClient: httpClient, ResourcePolicy: ResourcePolicy{AllowedHosts: []string{parsed.Hostname()}}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	return client, httpClient
}
