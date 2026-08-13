package pixiv_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

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
		if req.URL.Path != "/v2/illust/comments" {
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
		body := `{"comments":[{"id":7002,"comment":"hello","created_at":"2026-01-01T00:00:00Z","user":{"id":7,"name":"commenter"}}],"total_comments":3,"access_control":{"can_comment":true,"is_locked":false},"next_url":"https://app-api.pixiv.net/v2/illust/comments?illust_id=7001&offset=4"}`
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

func TestNovelCommentsReturnEmptyPageWithoutInventedMetadata(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v2/novel/comments" || req.URL.Query().Get("novel_id") != "8001" {
			t.Errorf("request = %s", req.URL.String())
		}
		body := `{"comments":[],"next_url":null}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	page, err := client.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 8001})
	if err != nil {
		t.Fatalf("NovelComments: %v", err)
	}
	if page.Page.Items == nil || len(page.Page.Items) != 0 || page.Total != nil || page.AccessControl != nil || !page.Page.Next.IsZero() {
		t.Fatalf("page = %#v", page)
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
