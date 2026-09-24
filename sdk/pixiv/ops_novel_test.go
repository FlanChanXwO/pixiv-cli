package pixiv_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	. "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func TestNovelRankingWiresModeAndCursor(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/novel/ranking" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("filter") != "for_android" || query.Get("mode") != string(RankingModeWeek) {
			t.Errorf("query = %v", query)
		}
		if calls == 1 {
			if _, ok := query["offset"]; ok {
				t.Errorf("initial request must not include offset: %v", query)
			}
		} else if query.Get("offset") != "30" {
			t.Errorf("continuation offset = %q, want %q", query.Get("offset"), "30")
		}
		body := `{"novels":[{"id":7001,"title":"ranked novel","create_date":"2026-01-01T00:00:00Z","user":{"id":17,"name":"writer"}}],"next_url":"https://app-api.pixiv.net/v1/novel/ranking?filter=for_android&mode=week&offset=30"}`
		if calls == 2 {
			body = `{"novels":[],"next_url":null}`
		}
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := NovelRankingRequest{Mode: RankingModeWeek}
	page, err := client.NovelRanking(context.Background(), request)
	if err != nil {
		t.Fatalf("NovelRanking: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 7001 || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.NovelRanking(context.Background(), request)
	if err != nil {
		t.Fatalf("NovelRanking continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestNovelCommentsMapsCurrentAppAPICommentDate(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v2/novel/comments" {
			t.Errorf("path = %q, want %q", req.URL.Path, "/v2/novel/comments")
		}
		if req.URL.Query().Get("novel_id") != "9001" {
			t.Errorf("novel_id = %q, want %q", req.URL.Query().Get("novel_id"), "9001")
		}
		return jsonResponse(`{"comments":[{"id":9002,"comment":"current wire","date":"2026-01-02T03:04:05+00:00","user":{"id":7,"name":"commenter"}}],"next_url":null}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}

	page, err := client.NovelComments(context.Background(), NovelCommentsRequest{NovelID: 9001})
	if err != nil {
		t.Fatalf("NovelComments: %v", err)
	}
	want := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	if len(page.Page.Items) != 1 || page.Page.Items[0].ID != 9002 || !page.Page.Items[0].CreatedAt.Equal(want) {
		t.Fatalf("page = %#v, want comment date %s", page, want)
	}
}

func TestNovelRankingRejectsUnsupportedModeBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		t.Fatalf("unsupported mode reached transport: %s", req.URL)
		return nil, nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	_, err = client.NovelRanking(context.Background(), NovelRankingRequest{Mode: RankingMode("unsupported")})
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if calls != 0 {
		t.Fatalf("unsupported mode reached transport %d time(s)", calls)
	}
}

func TestRecommendedNovelsPreservesExplicitZeroContinuation(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/novel/recommended" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if calls == 1 {
			if _, ok := query["offset"]; ok {
				t.Errorf("initial request must not include offset: %v", query)
			}
		} else if query.Get("offset") != "0" {
			t.Errorf("continuation offset = %q, want %q", query.Get("offset"), "0")
		}
		body := `{"novels":[{"id":7101,"title":"recommended novel","create_date":"2026-01-03T00:00:00Z","user":{"id":19,"name":"writer"}}],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?offset=0"}`
		if calls == 2 {
			body = `{"novels":[],"next_url":null}`
		}
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := RecommendedNovelsRequest{}
	page, err := client.RecommendedNovels(context.Background(), request)
	if err != nil {
		t.Fatalf("RecommendedNovels: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 7101 || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.RecommendedNovels(context.Background(), request)
	if err != nil {
		t.Fatalf("RecommendedNovels continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestFollowingNovelsWiresRestrictAndCursor(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/novel/follow" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("restrict") != string(RestrictPublic) {
			t.Errorf("restrict = %q, want %q", query.Get("restrict"), RestrictPublic)
		}
		wantOffset := ""
		if calls == 2 {
			wantOffset = "30"
		}
		if query.Get("offset") != wantOffset {
			t.Errorf("offset = %q, want %q", query.Get("offset"), wantOffset)
		}
		body := `{"novels":[{"id":7201,"title":"followed novel","create_date":"2026-01-04T00:00:00Z","user":{"id":20,"name":"followed writer"}}],"next_url":"https://app-api.pixiv.net/v1/novel/follow?restrict=public&offset=30"}`
		if calls == 2 {
			body = `{"novels":[],"next_url":null}`
		}
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := FollowingNovelsRequest{Restrict: RestrictPublic}
	page, err := client.FollowingNovels(context.Background(), request)
	if err != nil {
		t.Fatalf("FollowingNovels: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 7201 || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	request.Cursor = page.Next
	page, err = client.FollowingNovels(context.Background(), request)
	if err != nil {
		t.Fatalf("FollowingNovels continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
}

func TestFollowingNovelsDefaultsEmptyRestrictAndRejectsUnknownBeforeNetwork(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/novel/follow" {
			t.Errorf("path = %q", req.URL.Path)
		}
		if got := req.URL.Query().Get("restrict"); got != string(RestrictPublic) {
			t.Errorf("restrict = %q, want %q", got, RestrictPublic)
		}
		return jsonResponse(`{"novels":[],"next_url":null}`), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	if _, err := client.FollowingNovels(context.Background(), FollowingNovelsRequest{}); err != nil {
		t.Fatalf("FollowingNovels default restrict: %v", err)
	}
	_, err = client.FollowingNovels(context.Background(), FollowingNovelsRequest{Restrict: Restrict("unsupported")})
	if sdk.ReasonOf(err) != sdk.InvalidArgument {
		t.Fatalf("ReasonOf = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidArgument, err)
	}
	if calls != 1 {
		t.Fatalf("unknown restrict reached transport %d time(s)", calls-1)
	}
}

func TestLatestNovelsUsesMaxNovelIDContinuation(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/novel/new" {
			t.Errorf("path = %q", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("filter") != "for_android" {
			t.Errorf("filter = %q, want %q", query.Get("filter"), "for_android")
		}
		if _, ok := query["offset"]; ok {
			t.Errorf("latest novel request must not include offset: %v", query)
		}
		wantMaxNovelID := ""
		if calls == 2 {
			wantMaxNovelID = "987654"
		}
		if query.Get("max_novel_id") != wantMaxNovelID {
			t.Errorf("max_novel_id = %q, want %q", query.Get("max_novel_id"), wantMaxNovelID)
		}
		body := `{"novels":[{"id":8001,"title":"latest novel","create_date":"2026-01-02T00:00:00Z","user":{"id":18,"name":"new writer"}}],"next_url":"https://app-api.pixiv.net/v1/novel/new?filter=for_android&max_novel_id=987654"}`
		if calls == 2 {
			body = `{"novels":[],"next_url":null}`
		}
		return jsonResponse(body), nil
	})
	client, err := NewWith("token", Options{HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	request := LatestNovelsRequest{}
	page, err := client.LatestNovels(context.Background(), request)
	if err != nil {
		t.Fatalf("LatestNovels: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 8001 || page.Next.IsZero() {
		t.Fatalf("first page = %#v", page)
	}
	firstCursor := page.Next
	request.Cursor = page.Next
	page, err = client.LatestNovels(context.Background(), request)
	if err != nil {
		t.Fatalf("LatestNovels continuation: %v", err)
	}
	if page.Items == nil || len(page.Items) != 0 || !page.Next.IsZero() || calls != 2 {
		t.Fatalf("second page = %#v calls=%d", page, calls)
	}
	_, err = client.LatestNovels(context.Background(), LatestNovelsRequest{
		Cursor: cursorWithPayload(t, firstCursor, `{"k":"offset","v":30}`),
	})
	if sdk.ReasonOf(err) != sdk.InvalidCursor {
		t.Fatalf("legacy offset cursor reason = %q, want %q (err=%v)", sdk.ReasonOf(err), sdk.InvalidCursor, err)
	}
	if calls != 2 {
		t.Fatalf("legacy offset cursor reached transport %d time(s)", calls-2)
	}
}
