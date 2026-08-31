package pixiv_test

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"

	"strings"
	"testing"

	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func oauthResponse() *http.Response {
	body := `{"access_token":"new-access-token","refresh_token":"new-refresh-token","expires_in":3600,"user":{"id":42,"name":"tester"}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func oauthResponseWithoutIdentity() *http.Response {
	body := `{"access_token":"new-access-token","refresh_token":"new-refresh-token","expires_in":3600,"user":{"name":"tester"}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNewRejectsEmptyToken(t *testing.T) {
	if _, err := New(""); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
	if _, err := NewWith("  ", Options{}); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestOpenRotatesCredentials(t *testing.T) {
	var captured url.Values
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "oauth.secure.pixiv.net" {
			_ = req.ParseForm()
			captured = req.PostForm
			return oauthResponse(), nil
		}
		return nil, errors.New("unexpected host " + req.URL.Host)
	})
	httpClient := &http.Client{Transport: rt}
	client, creds, err := OpenWith(context.Background(), "old-refresh-token", Options{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if client.UserID() != 42 || client.Username() != "tester" {
		t.Fatalf("client identity = %d/%q", client.UserID(), client.Username())
	}
	if creds.AccessToken() != "new-access-token" || creds.RefreshToken() != "new-refresh-token" {
		t.Fatalf("credentials not rotated: access=%q refresh=%q", creds.AccessToken(), creds.RefreshToken())
	}
	if creds.ExpiresAt.IsZero() {
		t.Fatal("expiry not captured")
	}
	if captured.Get("grant_type") != "refresh_token" {
		t.Fatalf("grant_type = %q", captured.Get("grant_type"))
	}
}

func TestCurrentUserUsesVerifiedOAuthIdentity(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "oauth.secure.pixiv.net" {
			return oauthResponse(), nil
		}
		if req.URL.Host != "app-api.pixiv.net" || req.URL.Path != "/v1/user/detail" {
			return nil, errors.New("unexpected request " + req.URL.String())
		}
		if req.URL.Query().Get("user_id") != "42" || req.URL.Query().Get("filter") != "for_android" {
			return nil, errors.New("current user query does not use verified identity")
		}
		if req.Header.Get("X-User-Id") != "42" {
			return nil, errors.New("current user header does not use verified identity")
		}
		return jsonResponse(`{"user":{"id":42,"name":"tester"},"profile":{},"profile_publicity":{"gender":false,"region":false,"birth_day":false,"birth_year":false,"job":false,"pawoo":false},"workspace":{}}`), nil
	})
	client, _, err := OpenWith(context.Background(), "old-refresh-token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("OpenWith: %v", err)
	}
	result, err := client.CurrentUser(context.Background(), CurrentUserRequest{})
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if result.User.ID != 42 || result.User.Name != "tester" {
		t.Fatalf("current user = %#v", result)
	}
}

func TestOpenRejectsMissingAccountIdentity(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "oauth.secure.pixiv.net" {
			return oauthResponseWithoutIdentity(), nil
		}
		return nil, errors.New("unexpected host " + req.URL.Host)
	})
	client, credentials, err := OpenWith(context.Background(), "old-refresh-token", Options{HTTPClient: &http.Client{Transport: rt}})
	if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
		t.Fatalf("expected MalformedUpstreamResponse, got %v", err)
	}
	if client != nil {
		t.Fatal("client must not be returned without verified account identity")
	}
	if credentials.UserID != 0 || credentials.AccessToken() != "" || credentials.RefreshToken() != "" {
		t.Fatalf("credentials must be empty on malformed response: %#v", credentials)
	}
}

func TestOpenRejectsEmptyRefreshToken(t *testing.T) {
	if _, _, err := Open(context.Background(), ""); sdk.ReasonOf(err) != sdk.CredentialsExpired {
		t.Fatalf("expected CredentialsExpired, got %v", err)
	}
}

func TestSearchNovelsWiresQueryAndCursor(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/search/novel" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("word") != "novel" || query.Get("search_target") != string(SearchTargetKeyword) ||
			query.Get("sort") != string(SortModePopularDesc) || query.Get("duration") != string(DurationLastWeek) {
			t.Errorf("query = %v", query)
		}
		wantOffset := ""
		if calls == 2 {
			wantOffset = "30"
		}
		if query.Get("offset") != wantOffset {
			t.Errorf("offset = %q, want %q", query.Get("offset"), wantOffset)
		}
		body := `{"novels":[{"id":2001,"title":"novel","create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"writer"},"x_restrict":0,"text_length":12,"is_original":true,"tags":[]}],"next_url":"https://app-api.pixiv.net/v1/search/novel?word=novel&offset=30"}`
		if calls == 2 {
			body = `{"novels":[],"next_url":null}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := SearchNovelsRequest{
		Word: "novel", Target: SearchTargetKeyword, Sort: SortModePopularDesc, Duration: DurationLastWeek,
	}
	page, err := client.SearchNovels(context.Background(), request)
	if err != nil {
		t.Fatalf("SearchNovels: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 2001 || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.SearchNovels(context.Background(), request)
	if err != nil {
		t.Fatalf("SearchNovels continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestSearchUsersWiresQueryAndCursor(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/search/user" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("word") != "artist" {
			t.Errorf("word = %q", query.Get("word"))
		}
		wantOffset := ""
		if calls == 2 {
			wantOffset = "20"
		}
		if query.Get("offset") != wantOffset {
			t.Errorf("offset = %q, want %q", query.Get("offset"), wantOffset)
		}
		body := `{"user_previews":[{"user":{"id":3001,"name":"artist"}}],"next_url":"https://app-api.pixiv.net/v1/search/user?word=artist&offset=20"}`
		if calls == 2 {
			body = `{"user_previews":[],"next_url":null}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := SearchUsersRequest{Word: "artist"}
	page, err := client.SearchUsers(context.Background(), request)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].User.ID != 3001 || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.SearchUsers(context.Background(), request)
	if err != nil {
		t.Fatalf("SearchUsers continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestArtworkRankingWiresModeDateAndCursor(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/illust/ranking" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("mode") != string(RankingModeWeek) || query.Get("date") != "2026-01-31" {
			t.Errorf("query = %v", query)
		}
		wantOffset := ""
		if calls == 2 {
			wantOffset = "30"
		}
		if query.Get("offset") != wantOffset {
			t.Errorf("offset = %q, want %q", query.Get("offset"), wantOffset)
		}
		body := `{"illusts":[{"id":4001,"title":"ranked","type":"illust","create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}],"next_url":"https://app-api.pixiv.net/v1/illust/ranking?mode=week&offset=30"}`
		if calls == 2 {
			body = `{"illusts":[],"next_url":null}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := ArtworkRankingRequest{Mode: RankingModeWeek, Date: "2026-01-31"}
	page, err := client.ArtworkRanking(context.Background(), request)
	if err != nil {
		t.Fatalf("ArtworkRanking: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 4001 || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.ArtworkRanking(context.Background(), request)
	if err != nil {
		t.Fatalf("ArtworkRanking continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestArtworkSeriesWiresCursor(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/illust/series" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("illust_series_id") != "5001" {
			t.Errorf("series id = %q", query.Get("illust_series_id"))
		}
		wantLastOrder := ""
		if calls == 2 {
			wantLastOrder = "8"
		}
		if query.Get("last_order") != wantLastOrder {
			t.Errorf("last_order = %q, want %q", query.Get("last_order"), wantLastOrder)
		}
		body := `{"illust_series_detail":{"user":{"id":7,"name":"artist"}},"illusts":[{"id":5002,"title":"chapter","type":"manga","create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}],"next_url":"https://app-api.pixiv.net/v1/illust/series?illust_series_id=5001&last_order=8"}`
		if calls == 2 {
			body = `{"illust_series_detail":{"user":{"id":7,"name":"artist"}},"illusts":[],"next_url":null}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := ArtworkSeriesRequest{SeriesID: 5001}
	page, err := client.ArtworkSeries(context.Background(), request)
	if err != nil {
		t.Fatalf("ArtworkSeries: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 5002 || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.ArtworkSeries(context.Background(), request)
	if err != nil {
		t.Fatalf("ArtworkSeries continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestNovelSeriesWiresCursorAndMetadata(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/novel/series" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("series_id") != "6001" {
			t.Errorf("series id = %q", query.Get("series_id"))
		}
		wantLastOrder := ""
		if calls == 2 {
			wantLastOrder = "9"
		}
		if query.Get("last_order") != wantLastOrder {
			t.Errorf("last_order = %q, want %q", query.Get("last_order"), wantLastOrder)
		}
		body := `{"novel_series_detail":{"id":6001,"title":"series","caption":"caption","is_concluded":true,"user":{"id":8,"name":"writer"}},"novels":[{"id":6002,"title":"chapter","create_date":"2026-01-01T00:00:00Z","user":{"id":8,"name":"writer"}}],"next_url":"https://app-api.pixiv.net/v1/novel/series?series_id=6001&last_order=9"}`
		if calls == 2 {
			body = `{"novel_series_detail":{"id":6001,"title":"series","user":{"id":8,"name":"writer"}},"novels":[],"next_url":null}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := NovelSeriesRequest{SeriesID: 6001}
	result, err := client.NovelSeries(context.Background(), request)
	if err != nil {
		t.Fatalf("NovelSeries: %v", err)
	}
	if result.Series.ID != 6001 || result.Series.Title != "series" || !result.Series.IsConcluded ||
		result.Series.User.ID != 8 || len(result.Novels.Items) != 1 || result.Novels.Items[0].ID != 6002 || result.Novels.Next.IsZero() {
		t.Fatalf("first result = %#v", result)
	}
	request.Cursor = result.Novels.Next
	result, err = client.NovelSeries(context.Background(), request)
	if err != nil {
		t.Fatalf("NovelSeries continuation: %v", err)
	}
	if result.Novels.Items == nil || len(result.Novels.Items) != 0 || !result.Novels.Next.IsZero() || calls != 2 {
		t.Fatalf("second result = %#v calls=%d", result, calls)
	}
}

func TestArtworkCommentsPreserveMetadataAndCursor(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v3/illust/comments" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("illust_id") != "7001" {
			t.Errorf("illust_id = %q", query.Get("illust_id"))
		}
		wantOffset := ""
		if calls == 2 {
			wantOffset = "4"
		}
		if query.Get("offset") != wantOffset {
			t.Errorf("offset = %q, want %q", query.Get("offset"), wantOffset)
		}
		body := `{"comments":[{"id":7002,"comment":"hello","created_at":"2026-01-01T00:00:00Z","user":{"id":7,"name":"commenter"}}],"total_comments":3,"access_control":{"can_comment":true,"is_locked":false},"next_url":"https://app-api.pixiv.net/v3/illust/comments?illust_id=7001&offset=4"}`
		if calls == 2 {
			body = `{"comments":[],"next_url":null}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := ArtworkCommentsRequest{ArtworkID: 7001}
	page, err := client.ArtworkComments(context.Background(), request)
	if err != nil {
		t.Fatalf("ArtworkComments: %v", err)
	}
	if len(page.Page.Items) != 1 || page.Page.Items[0].ID != 7002 || page.Total == nil || *page.Total != 3 ||
		page.AccessControl == nil || !page.AccessControl.CanComment || page.Page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Page.Next
	page, err = client.ArtworkComments(context.Background(), request)
	if err != nil {
		t.Fatalf("ArtworkComments continuation: %v", err)
	}
	if page.Page.Items == nil || len(page.Page.Items) != 0 || !page.Page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestNovelAndUserDetailsWireOperation(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/novel/detail":
			if req.URL.Query().Get("novel_id") != "9001" {
				t.Errorf("novel query = %v", req.URL.Query())
			}
			body := `{"novel":{"id":9001,"title":"novel detail","create_date":"2026-01-01T00:00:00Z","user":{"id":9,"name":"writer"}}}`
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		case "/v1/user/detail":
			if req.URL.Query().Get("user_id") != "9002" {
				t.Errorf("user query = %v", req.URL.Query())
			}
			body := `{"user":{"id":9002,"name":"user"},"profile":{},"profile_publicity":{"gender":false,"region":false,"birth_day":false,"birth_year":false,"job":false,"pawoo":false},"workspace":{}}`
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		default:
			t.Errorf("unexpected path %q", req.URL.Path)
			return nil, errors.New("unexpected path")
		}
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	novel, err := client.Novel(context.Background(), NovelRequest{NovelID: 9001})
	if err != nil {
		t.Fatalf("Novel: %v", err)
	}
	if novel.ID != 9001 || novel.Title != "novel detail" || novel.User.ID != 9 {
		t.Fatalf("novel = %#v", novel)
	}
	user, err := client.User(context.Background(), UserRequest{UserID: 9002})
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if user.User.ID != 9002 || user.User.Name != "user" {
		t.Fatalf("user = %#v", user)
	}
}

func TestReadOperationsMapPermissionErrors(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	tests := []struct {
		name string
		call func() error
	}{
		{name: "artwork detail", call: func() error { _, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 1}); return err }},
		{name: "novel detail", call: func() error { _, err := client.Novel(context.Background(), NovelRequest{NovelID: 1}); return err }},
		{name: "user detail", call: func() error { _, err := client.User(context.Background(), UserRequest{UserID: 1}); return err }},
		{name: "ranking", call: func() error {
			_, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{})
			return err
		}},
		{name: "artwork comments", call: func() error {
			_, err := client.ArtworkComments(context.Background(), ArtworkCommentsRequest{ArtworkID: 1})
			return err
		}},
		{name: "novel comments", call: func() error {
			_, err := client.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 1})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); sdk.ReasonOf(err) != sdk.Forbidden {
				t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.Forbidden, err)
			}
		})
	}
}

func TestSearchArtworksWiresOperation(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "app-api.pixiv.net" {
			return nil, errors.New("unexpected host " + req.URL.Host)
		}
		if req.URL.Path != "/v1/search/illust" {
			t.Errorf("path = %s", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("word") != "test" {
			t.Errorf("word = %q, want test", query.Get("word"))
		}
		if query.Get("search_target") != string(SearchTargetPartialMatchForTags) {
			t.Errorf("search_target = %q, want %q", query.Get("search_target"), SearchTargetPartialMatchForTags)
		}
		if query.Get("sort") != string(SortModeDateDesc) {
			t.Errorf("sort = %q, want %q", query.Get("sort"), SortModeDateDesc)
		}
		offset := query.Get("offset")
		if calls == 1 && offset != "" {
			t.Errorf("first call should omit offset, got %q", offset)
		}
		if calls == 2 && offset != "30" {
			t.Errorf("second call should pass offset=30, got %q", offset)
		}
		body := `{"illusts":[{"id":9001,"title":"art","type":"illust","create_date":"2024-05-01T10:00:00+09:00","image_urls":{"original":"https://i.pximg.net/img/9001.png"},"user":{"id":7,"name":"n","account":"a"},"tags":[]}],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=test&search_target=partial_match_for_tags&offset=30"}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})

	page, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test", Target: SearchTargetPartialMatchForTags})
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 9001 {
		t.Fatalf("page items = %+v", page.Items)
	}
	if page.Next.IsZero() {
		t.Fatal("expected a continuation cursor")
	}

	page2, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test", Target: SearchTargetPartialMatchForTags, Cursor: page.Next})
	if err != nil {
		t.Fatalf("continuation SearchArtworks: %v", err)
	}
	if len(page2.Items) != 1 || calls != 2 {
		t.Fatalf("page2 items=%d calls=%d", len(page2.Items), calls)
	}
}

func TestSearchArtworksRejectsChangedQuery(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"illusts":[],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=test&offset=30"}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	page, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "test"})
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if page.Next.IsZero() {
		t.Fatal("expected cursor")
	}

	if _, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "different", Cursor: page.Next}); sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("expected InvalidCursor for changed query, got %v", err)
	}
}

// TestLatestArtworksBindsCursorToContentType 验证 finding #12：latest-artwork 游标
// 绑定 content type，一个为 illust 生成的 continuation 不得在 manga 请求里复用，
// 否则会在错误的 result set 上恢复 offset。
func TestLatestArtworksBindsCursorToContentType(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 2 && req.URL.Query().Get("max_illust_id") != "987654" {
			t.Errorf("continuation query = %v", req.URL.Query())
		}
		body := `{"illusts":[{"id":42,"title":"art","type":"illust","create_date":"2024-05-01T10:00:00+09:00","image_urls":{"original":"https://i.pximg.net/img/42.png"},"user":{"id":7,"name":"n","account":"a"},"tags":[]}],"next_url":"https://app-api.pixiv.net/v1/illust/new?content_type=illust&filter=for_android&max_illust_id=987654"}`
		return jsonResponse(body), nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	page, err := client.LatestArtworks(context.Background(), LatestArtworksRequest{ContentType: SearchContentTypeIllust})
	if err != nil {
		t.Fatalf("LatestArtworks: %v", err)
	}
	if page.Next.IsZero() {
		t.Fatal("expected continuation cursor")
	}
	// A cursor produced for the illust feed must be rejected for the manga feed.
	_, err = client.LatestArtworks(context.Background(), LatestArtworksRequest{ContentType: SearchContentTypeManga, Cursor: page.Next})
	if sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("expected InvalidCursor for changed content type, got %v", err)
	}
	// The same content type must continue successfully.
	if _, err := client.LatestArtworks(context.Background(), LatestArtworksRequest{ContentType: SearchContentTypeIllust, Cursor: page.Next}); err != nil {
		t.Fatalf("continuation LatestArtworks: %v", err)
	}
}

func TestArtworkWiresDetailPreservesPagesAndResources(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/illust/detail" {
			t.Errorf("path = %s", req.URL.Path)
		}
		body := `{"illust":{"id":5,"title":"one","type":"manga","create_date":"2024-01-01T00:00:00Z","page_count":2,"image_urls":{"original":"https://i.pximg.net/img/5.png"},"meta_pages":[{"page_index":0,"width":100,"height":100,"image_urls":{"original":"https://i.pximg.net/img/5_p0.png"}},{"page_index":1,"width":100,"height":100,"image_urls":{"original":"https://i.pximg.net/img/5_p1.png"}}],"user":{"id":9,"name":"u","account":"u"},"tags":[]}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	artwork, err := client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 5})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	if artwork.Kind != ArtworkKindManga || artwork.ID != 5 || len(artwork.Pages) != 2 {
		t.Fatalf("artwork = %+v", artwork)
	}
	if artwork.Cover.Resource.Ref.IsZero() || artwork.Pages[1].Image.Resource.Ref.IsZero() {
		t.Fatal("artwork resources lost opaque references")
	}
	if !strings.Contains(artwork.Pages[1].Image.Resource.URL, "5_p1.png") {
		t.Fatalf("page URL = %q", artwork.Pages[1].Image.Resource.URL)
	}
}

func TestPixivFamiliesNoWebFallbackTokenMatrix(t *testing.T) {
	families := []struct {
		name string
		call func(context.Context, *Client) error
	}{
		{name: "search", call: func(ctx context.Context, client *Client) error {
			_, err := client.SearchArtworks(ctx, SearchArtworksRequest{Word: "representative"})
			return err
		}},
		{name: "detail", call: func(ctx context.Context, client *Client) error {
			_, err := client.Artwork(ctx, ArtworkRequest{ArtworkID: 1})
			return err
		}},
		{name: "bookmark", call: func(ctx context.Context, client *Client) error {
			_, err := client.ArtworkBookmark(ctx, ArtworkBookmarkRequest{ArtworkID: 1})
			return err
		}},
		{name: "mutation", call: func(ctx context.Context, client *Client) error {
			return client.AddBookmark(ctx, AddBookmarkRequest{ArtworkID: 1})
		}},
	}
	for _, family := range families {
		t.Run(family.name, func(t *testing.T) {
			calls := 0
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Host != "app-api.pixiv.net" {
					return nil, errors.New("unexpected non-App API host")
				}
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			client, err := NewWith("access-token", Options{HTTPClient: &http.Client{Transport: rt}})
			if err != nil {
				t.Fatalf("NewWith: %v", err)
			}
			if err := family.call(context.Background(), client); sdk.ReasonOf(err) != sdk.CredentialsExpired {
				t.Fatalf("error reason = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.CredentialsExpired, err)
			}
			if calls != 1 {
				t.Fatalf("family made %d App API calls, want 1", calls)
			}
		})
	}
}

func TestAddBookmarkWiresMutation(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "app-api.pixiv.net" || req.URL.Path != "/v2/illust/bookmark/add" {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.String())
		}
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	client, _ := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err := client.AddBookmark(context.Background(), AddBookmarkRequest{ArtworkID: 12, Tags: []string{"tag"}}); err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
}

func TestSearchArtworksRejectsInvalidBookmarkRange(t *testing.T) {
	client, err := New("token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	minimum, maximum := 10, 5
	tests := []struct {
		name string
		min  *int
		max  *int
	}{
		{name: "negative minimum", min: pointer(-1)},
		{name: "negative maximum", max: pointer(-1)},
		{name: "minimum exceeds maximum", min: &minimum, max: &maximum},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{
				Word:        "range",
				BookmarkMin: test.min,
				BookmarkMax: test.max,
			})
			if sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want %q", sdk.ReasonOf(err), sdk.InvalidArgument)
			}
		})
	}
}

func pointer(value int) *int {
	return &value
}

func TestSearchRejectsUnknownQueryValues(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client) error
	}{
		{name: "artwork filter", call: func(client *Client) error {
			_, err := client.SearchArtworks(context.Background(), SearchArtworksRequest{Word: "query", Target: SearchTarget("unknown")})
			return err
		}},
		{name: "novel filter", call: func(client *Client) error {
			_, err := client.SearchNovels(context.Background(), SearchNovelsRequest{Word: "query", Target: SearchTarget("unknown")})
			return err
		}},
		{name: "user whitespace", call: func(client *Client) error {
			_, err := client.SearchUsers(context.Background(), SearchUsersRequest{Word: " \t"})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := New("token")
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if err := test.call(client); sdk.ReasonOf(err) != sdk.InvalidArgument {
				t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
			}
		})
	}
}

func TestSearchArtworksAIModeOnlyFiltersBatchAndBindsCursor(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		query := req.URL.Query()
		if query.Get("search_ai_type") != "0" {
			t.Errorf("search_ai_type = %q, want 0 for local only filtering", query.Get("search_ai_type"))
		}
		body := `{"illusts":[
			{"id":901,"title":"ai","type":"illust","ai_type":2,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}},
			{"id":902,"title":"human","type":"illust","ai_type":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}
		],"next_url":"https://app-api.pixiv.net/v1/search/illust?word=tag&offset=30"}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := SearchArtworksRequest{Word: "tag", AIMode: SearchAIModeOnly}
	page, err := client.SearchArtworks(context.Background(), request)
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 901 || page.Items[0].AIType != 2 {
		t.Fatalf("filtered items = %#v, want only AI artwork", page.Items)
	}
	if page.Next.IsZero() {
		t.Fatal("expected continuation cursor")
	}

	_, err = client.SearchArtworks(context.Background(), SearchArtworksRequest{
		Word: "tag", AIMode: SearchAIModeExclude, Cursor: page.Next,
	})
	if sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("ReasonOf = %q, want %q (calls=%d err=%v)", sdk.ReasonOf(err), sdk.InvalidCursor, calls, err)
	}
	if calls != 1 {
		t.Fatalf("changed AI mode reached upstream; calls = %d, want 1", calls)
	}
}

func TestArtworkRankingDefaultsAndValidatesMode(t *testing.T) {
	client, err := New("token")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: RankingMode("unknown")}); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("invalid mode ReasonOf = %q, want %q", sdk.ReasonOf(err), sdk.InvalidArgument)
	}
	if _, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{Mode: RankingModeDay, Date: "2026-02-30"}); sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("invalid date ReasonOf = %q, want %q", sdk.ReasonOf(err), sdk.InvalidArgument)
	}

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.URL.Query().Get("mode"); got != string(RankingModeDay) {
			t.Errorf("mode = %q, want %q", got, RankingModeDay)
		}
		body := `{"illusts":[],"next_url":null}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err = NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	page, err := client.ArtworkRanking(context.Background(), ArtworkRankingRequest{})
	if err != nil {
		t.Fatalf("ArtworkRanking: %v", err)
	}
	if page.Items == nil {
		t.Fatal("successful empty ranking must contain a non-nil Items slice")
	}
}

// TestPublicInventoryNoRawMediaURLFields guards the invariant that media URLs
// never appear as loose public fields; they may only live inside Resource.URL.
func TestPublicInventoryNoRawMediaURLFields(t *testing.T) {
	forbidden := []string{"DownloadURL", "ZipURLs", "OriginalURL", "SignedURL", "ImageURLs", "MetaSinglePage", "MetaPages"}
	clientType := reflect.TypeOf((*Client)(nil))
	for i := 0; i < clientType.NumMethod(); i++ {
		method := clientType.Method(i)
		found := scanTypeForFields(method.Type, forbidden)
		if len(found) > 0 {
			t.Errorf("method %s exposes forbidden media URL fields: %v", method.Name, found)
		}
	}
}

func scanTypeForFields(typ reflect.Type, forbidden []string) []string {
	if typ.Kind() == reflect.Pointer {
		return scanTypeForFields(typ.Elem(), forbidden)
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	var found []string
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		for _, name := range forbidden {
			if field.Name == name {
				found = append(found, field.Name)
			}
		}
		if field.Type.Kind() == reflect.Struct || field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array {
			if next := scanTypeForFields(field.Type, forbidden); len(next) > 0 {
				found = append(found, next...)
			}
		}
	}
	return found
}

func TestArtworkPublicMappingRejectsMissingPublishTime(t *testing.T) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"illust":{"id":1,"type":"illust","image_urls":{"original":"https://i.pximg.net/img/1.png"},"user":{"id":7,"name":"artist"}}}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	_, err = client.Artwork(context.Background(), ArtworkRequest{ArtworkID: 1})
	if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
		t.Fatalf("reason = %q, want %q", sdk.ReasonOf(err), sdk.MalformedUpstreamResponse)
	}
}

func TestArtworkPagesPublicMappingDerivesSinglePage(t *testing.T) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"illust":{"id":5,"type":"illust","create_date":"2024-01-01T00:00:00Z","image_urls":{"original":"https://i.pximg.net/img/5.png"},"meta_single_page":{"original_image_url":"https://i.pximg.net/img/5.png"},"user":{"id":7,"name":"artist"}}}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	pages, err := client.ArtworkPages(context.Background(), ArtworkPagesRequest{ArtworkID: 5})
	if err != nil {
		t.Fatalf("ArtworkPages: %v", err)
	}
	if len(pages) != 1 || pages[0].PageIndex != 0 || !strings.Contains(pages[0].Image.Resource.URL, "5.png") {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestUgoiraMetadataMapsFramesAndRejectsUnsafeFilename(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(`{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/zip/m.zip","original":"https://i.pximg.net/zip/o.zip"},"frames":[{"file":"0.jpg","delay":100},{"file":"1.jpg","delay":200}]}}`), nil
		})
		client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
		if err != nil {
			t.Fatalf("NewWith: %v", err)
		}
		meta, err := client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 777})
		if err != nil || meta.ArtworkID != 777 || len(meta.Archives) != 2 || len(meta.Frames) != 2 || meta.Frames[1].DelayMilliseconds != 200 {
			t.Fatalf("metadata = %+v err=%v", meta, err)
		}
	})

	t.Run("unsafe filename", func(t *testing.T) {
		rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(`{"ugoira_metadata":{"zip_urls":{"original":"https://i.pximg.net/zip/o.zip"},"frames":[{"file":"../evil.jpg","delay":100}]}}`), nil
		})
		client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
		if err != nil {
			t.Fatalf("NewWith: %v", err)
		}
		_, err = client.UgoiraMetadata(context.Background(), UgoiraMetadataRequest{ArtworkID: 1})
		if sdk.ReasonOf(err) != sdk.MalformedUpstreamResponse {
			t.Fatalf("reason = %q, want %q", sdk.ReasonOf(err), sdk.MalformedUpstreamResponse)
		}
	})
}

func TestNovelContentPublicParserPreservesUnknownBlock(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/novel/content" {
			t.Fatalf("path = %q", req.URL.Path)
		}
		body := `<html><body><div class="novel-view"><div class="novel-body"><p class="noveltext">known text</p><div class="novel_something">unknown block payload</div></div></div></body></html>`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	content, err := client.NovelContent(context.Background(), NovelContentRequest{NovelID: 1})
	if err != nil {
		t.Fatalf("NovelContent: %v", err)
	}
	if len(content.Blocks) != 2 || content.Blocks[1].Kind != NovelBlockUnknown || content.Blocks[1].Unknown == nil {
		t.Fatalf("content = %+v", content)
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

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
		if req.URL.Path != "/v2/illust/bookmark/detail" || req.URL.Query().Get("illust_id") != "77" {
			t.Errorf("request = %s?%s", req.URL.Path, req.URL.RawQuery)
		}
		body := `{"bookmark_detail":{"is_bookmarked":true,"restrict":"private","tags":[{"name":"cat","is_registered":true},{"name":"fav","is_registered":false}]}}`
		if calls == 2 {
			body = `{"bookmark_detail":{"is_bookmarked":false,"restrict":"public","tags":[{"name":"cat","is_registered":false}]}}`
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
