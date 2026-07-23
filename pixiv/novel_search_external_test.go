package pixiv_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestAuthenticatedSearchNovelMapsSearchFields(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search/novel" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access" {
			t.Fatalf("authorization = %q", got)
		}
		query := r.URL.Query()
		if query.Get("word") != "miku" || query.Get("search_target") != "partial_match_for_tags" || query.Get("sort") != "date_desc" || query.Get("duration") != "within_last_week" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"novels":[{"id":71,"title":"novel","caption":"description","user":{"id":8,"name":"author"},"tags":[],"image_urls":{},"create_date":"2026-07-23T00:00:00+00:00","total_bookmarks":12,"total_view":34,"x_restrict":1,"text_length":2400,"is_original":true}],"next_url":"/v1/search/novel?offset=30"}`)
	}))
	defer server.Close()

	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.SearchNovel(context.Background(), pixiv.SearchNovelRequest{
		Word: "miku", Target: pixiv.SearchTargetPartialMatchForTags, Sort: pixiv.SortModeDateDesc, Duration: "within_last_week",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Novels) != 1 {
		t.Fatalf("novels = %#v", result.Novels)
	}
	novel := result.Novels[0]
	if novel.ID != 71 || novel.URL != "https://www.pixiv.net/novel/show.php?id=71" || novel.Caption != "description" || novel.XRestrict != 1 || novel.TextLength != 2400 || !novel.IsOriginal {
		t.Fatalf("novel = %#v", novel)
	}
	if result.NextCursor == "" {
		t.Fatal("next cursor is empty")
	}
}

func TestSearchNovelFiltersByStableResponseFields(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		for _, key := range []string{"x_restrict", "rating", "text_length", "is_original"} {
			if got := r.URL.Query().Get(key); got != "" {
				t.Fatalf("unexpected invented query %s=%q", key, got)
			}
		}
		switch r.URL.Query().Get("offset") {
		case "":
			fmt.Fprint(w, `{"novels":[{"id":1,"user":{"id":1},"x_restrict":0,"text_length":100,"is_original":false}],"next_url":"/v1/search/novel?offset=30"}`)
		case "30":
			fmt.Fprint(w, `{"novels":[{"id":2,"user":{"id":2},"x_restrict":1,"text_length":300,"is_original":true}],"next_url":null}`)
		default:
			t.Fatalf("offset = %q", r.URL.Query().Get("offset"))
		}
	}))
	defer server.Close()

	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access"})
	if err != nil {
		t.Fatal(err)
	}

	first, err := client.SearchNovel(context.Background(), pixiv.SearchNovelRequest{
		Word: "miku",
		Filters: pixiv.NovelSearchFilters{
			Rating: pixiv.SearchRatingR18, MinTextLength: 200, MaxTextLength: 400, OriginalOnly: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Novels) != 0 || first.NextCursor == "" {
		t.Fatalf("first result = %#v", first)
	}

	second, err := client.SearchNovel(context.Background(), pixiv.SearchNovelRequest{
		Word: "miku", Cursor: first.NextCursor,
		Filters: pixiv.NovelSearchFilters{
			Rating: pixiv.SearchRatingR18, MinTextLength: 200, MaxTextLength: 400, OriginalOnly: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(second.Novels) != 1 || second.Novels[0].ID != 2 || second.NextCursor != "" {
		t.Fatalf("requests=%d second=%#v", requests, second)
	}
}

func TestSearchNovelRejectsMissingFilterFields(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"novels":[{"id":1,"user":{"id":1},"x_restrict":0,"is_original":true}],"next_url":null}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.SearchNovel(context.Background(), pixiv.SearchNovelRequest{Word: "miku"})
	if result != nil || !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Operation != pixiv.OperationSearchNovel || typed.Backend != pixiv.BackendAppAPI {
		t.Fatalf("typed error = %#v", typed)
	}
}

func TestSearchNovelRequiresAuthenticationBeforeNetwork(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, "unexpected network request", http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.SearchNovel(context.Background(), pixiv.SearchNovelRequest{Word: "miku"})
	if result != nil || !errors.Is(err, pixiv.ErrUnauthorized) {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Operation != pixiv.OperationSearchNovel || typed.Backend != "" || typed.UpstreamStatus != 0 {
		t.Fatalf("typed error = %#v", typed)
	}
	if requests != 0 {
		t.Fatalf("requests=%d, want no anonymous fallback request", requests)
	}
}

func TestSearchUserReportsOfficialAppSource(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search/user" || r.URL.Query().Get("word") != "author" {
			t.Fatalf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"user_previews":[{"user":{"id":8,"name":"author","account":"account","comment":"profile"}}],"next_url":null}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "access"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.SearchUser(context.Background(), pixiv.SearchUserRequest{Word: "author"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != pixiv.UserSearchSourceApp || len(result.UserPreviews) != 1 || result.UserPreviews[0].User.Comment != "profile" {
		t.Fatalf("result = %#v", result)
	}
}
