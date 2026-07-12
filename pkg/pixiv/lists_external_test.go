package pixiv_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
)

func TestSearchIllustAppCursorContinuesAndEndsWithoutSecrets(t *testing.T) {
	t.Parallel()
	const secret = "cursor-must-not-contain-this"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Errorf("Authorization = %q", got)
		}
		switch r.URL.Query().Get("offset") {
		case "":
			fmt.Fprintf(w, `{"illusts":[{"id":101,"title":"first","user":{"id":9}}],"next_url":"https://app-api.pixiv.net/v1/search/illust?offset=30&access_token=%s"}`, secret)
		case "30":
			fmt.Fprint(w, `{"illusts":[],"next_url":null}`)
		default:
			t.Errorf("unexpected offset %q", r.URL.Query().Get("offset"))
		}
	}))
	defer server.Close()

	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: secret})
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku"})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Illusts) != 1 || first.Illusts[0].ID != 101 || first.NextCursor == "" {
		t.Fatalf("first = %#v", first)
	}
	if strings.Contains(string(first.NextCursor), secret) || strings.Contains(string(first.NextCursor), "miku") || strings.Contains(string(first.NextCursor), "next_url") {
		t.Fatalf("cursor leaks source data: %q", first.NextCursor)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(string(first.NextCursor))
	if err != nil {
		t.Fatalf("cursor is not base64url: %v", err)
	}
	if strings.Contains(string(decoded), secret) || strings.Contains(string(decoded), "miku") || strings.Contains(string(decoded), "next_url") || strings.Contains(string(decoded), "access_token") {
		t.Fatalf("decoded cursor leaks source data: %s", decoded)
	}
	second, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Illusts) != 0 || second.NextCursor != "" || requests.Load() != 2 {
		t.Fatalf("second = %#v; requests=%d", second, requests.Load())
	}
}

func TestCursorMismatchAndMalformedFailBeforeNetwork(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{"illusts":[],"next_url":"/v1/search/illust?offset=30"}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku"})
	if err != nil {
		t.Fatal(err)
	}
	if first.NextCursor == "" {
		t.Fatal("missing cursor")
	}
	baseline := requests.Load()

	cases := []struct {
		name string
		call func() error
	}{
		{"changed query", func() error {
			_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "rin", Cursor: first.NextCursor})
			return err
		}},
		{"cross operation", func() error {
			_, err := client.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Cursor: first.NextCursor})
			return err
		}},
		{"invalid base64", func() error {
			_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: "%%%"})
			return err
		}},
		{"unknown version", func() error {
			_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: encodedCursorFixture(`{"v":2,"o":"search_illust","q":"x","k":"offset","n":"1"}`)})
			return err
		}},
		{"unknown kind", func() error {
			_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: encodedCursorFixture(`{"v":1,"o":"search_illust","q":"x","k":"url","n":"1"}`)})
			return err
		}},
		{"illegal number", func() error {
			_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: encodedCursorFixture(`{"v":1,"o":"search_illust","q":"x","k":"offset","n":"-1"}`)})
			return err
		}},
		{"trailing data", func() error {
			_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: encodedCursorFixture(`{"v":1,"o":"search_illust","q":"x","k":"offset","n":"1"} {}`)})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, pixiv.ErrInvalidArgument) {
				t.Fatalf("error = %v, want invalid_argument", err)
			}
			var typed *pixiv.Error
			if !errors.As(err, &typed) || typed.Backend != "" {
				t.Fatalf("typed error = %#v", typed)
			}
			if requests.Load() != baseline {
				t.Fatalf("network requests = %d, want %d", requests.Load(), baseline)
			}
		})
	}
}

func encodedCursorFixture(raw string) pixiv.Cursor {
	return pixiv.Cursor(base64.RawURLEncoding.EncodeToString([]byte(raw)))
}

func TestAuthenticatedListOperationsUseExpectedAppEndpointsAndContinuations(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("missing App authorization")
		}
		switch r.URL.Path {
		case "/v1/illust/ranking":
			if r.URL.Query().Get("mode") != "day" {
				t.Errorf("ranking mode = %q", r.URL.Query().Get("mode"))
			}
			fmt.Fprint(w, `{"illusts":[{"id":1}],"next_url":"/v1/illust/ranking?offset=30"}`)
		case "/v1/illust/recommended":
			fmt.Fprint(w, `{"illusts":[{"id":2}],"next_url":"/v1/illust/recommended?offset=30"}`)
		case "/v2/illust/follow":
			if r.URL.Query().Get("restrict") != "public" {
				t.Errorf("follow restrict = %q", r.URL.Query().Get("restrict"))
			}
			fmt.Fprint(w, `{"illusts":[{"id":3}],"next_url":"/v2/illust/follow?offset=30"}`)
		case "/v1/search/user":
			fmt.Fprint(w, `{"user_previews":[{"user":{"id":4,"name":"searched"}}],"next_url":"/v1/search/user?offset=30"}`)
		case "/v1/user/detail":
			if r.URL.Query().Get("user_id") != "42" {
				t.Errorf("detail user_id = %q", r.URL.Query().Get("user_id"))
			}
			fmt.Fprint(w, `{"user":{"id":42,"name":"detail"}}`)
		case "/v1/user/illusts":
			if r.URL.Query().Get("user_id") != "42" || r.URL.Query().Get("type") != "illust" {
				t.Errorf("artworks query = %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"illusts":[{"id":5}],"next_url":"/v1/user/illusts?offset=30"}`)
		case "/v1/user/bookmarks/illust":
			if r.URL.Query().Get("user_id") != "42" || r.URL.Query().Get("restrict") != "public" || r.URL.Query().Get("tag") != "cat" {
				t.Errorf("bookmarks query = %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"illusts":[{"id":6}],"next_url":"/v1/user/bookmarks/illust?max_bookmark_id=777"}`)
		case "/v1/user/following":
			if r.URL.Query().Get("user_id") != "42" || r.URL.Query().Get("restrict") != "public" {
				t.Errorf("following query = %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"user_previews":[{"user":{"id":7}}],"next_url":"/v1/user/following?offset=30"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	assertIllustBatch := func(name string, result *pixiv.IllustListResult, err error, id int64) {
		t.Helper()
		if err != nil || len(result.Illusts) != 1 || result.Illusts[0].ID != id || result.NextCursor == "" {
			t.Fatalf("%s = %#v, %v", name, result, err)
		}
	}
	ranking, err := client.IllustRanking(ctx, pixiv.IllustRankingRequest{})
	assertIllustBatch("ranking", ranking, err, 1)
	recommended, err := client.IllustRecommended(ctx, pixiv.IllustRecommendedRequest{})
	assertIllustBatch("recommended", recommended, err, 2)
	follow, err := client.FollowingIllusts(ctx, pixiv.FollowingIllustsRequest{})
	assertIllustBatch("follow", follow, err, 3)
	searched, err := client.SearchUser(ctx, pixiv.SearchUserRequest{Word: "alice"})
	if err != nil || len(searched.UserPreviews) != 1 || searched.UserPreviews[0].User.ID != 4 || searched.NextCursor == "" {
		t.Fatalf("search user = %#v, %v", searched, err)
	}
	detail, err := client.UserDetail(ctx, pixiv.UserDetailRequest{UserID: 42})
	if err != nil || detail.User.ID != 42 {
		t.Fatalf("user detail = %#v, %v", detail, err)
	}
	artworks, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: 42})
	assertIllustBatch("artworks", artworks, err, 5)
	bookmarks, err := client.UserBookmarks(ctx, pixiv.UserBookmarksRequest{UserID: 42, Tag: "cat"})
	assertIllustBatch("bookmarks", bookmarks, err, 6)
	following, err := client.UserFollowing(ctx, pixiv.UserFollowingRequest{UserID: 42})
	if err != nil || len(following.UserPreviews) != 1 || following.UserPreviews[0].User.ID != 7 || following.NextCursor == "" {
		t.Fatalf("following = %#v, %v", following, err)
	}
}

func TestAppListRejectsMissingItemsIDsAndMalformedContinuationSafely(t *testing.T) {
	t.Parallel()
	const secret = "do-not-leak-next-url-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("word") {
		case "missing":
			fmt.Fprint(w, `{}`)
		case "null":
			fmt.Fprint(w, `{"illusts":null}`)
		case "id":
			fmt.Fprint(w, `{"illusts":[{"title":"no id"}]}`)
		case "next":
			fmt.Fprintf(w, `{"illusts":[],"next_url":"/v1/search/illust?access_token=%s"}`, secret)
		case "empty":
			fmt.Fprint(w, `{"illusts":[]}`)
		}
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"missing", "null", "id", "next"} {
		result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: word})
		if result != nil || !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) {
			t.Errorf("%s result=%#v err=%v", word, result, err)
		}
		if strings.Contains(fmt.Sprint(err), secret) || strings.Contains(fmt.Sprint(errors.Unwrap(err)), secret) {
			t.Errorf("%s leaked secret: %v", word, err)
		}
		var typed *pixiv.Error
		if !errors.As(err, &typed) || typed.Operation != pixiv.OperationSearchIllust || typed.Backend != pixiv.BackendAppAPI {
			t.Errorf("%s metadata=%#v", word, typed)
		}
	}
	result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "empty"})
	if err != nil || result == nil || result.Illusts == nil || len(result.Illusts) != 0 {
		t.Fatalf("explicit empty = %#v, %v", result, err)
	}
}

func TestAnonymousWebSearchRejectsMissingItemsAndIDsButAcceptsEmpty(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("word") {
		case "missing":
			fmt.Fprint(w, `{"error":false,"body":{"illustManga":{}}}`)
		case "null":
			fmt.Fprint(w, `{"error":false,"body":{"illustManga":{"data":null}}}`)
		case "id":
			fmt.Fprint(w, `{"error":false,"body":{"illustManga":{"data":[{"title":"bad"}]}}}`)
		case "empty":
			fmt.Fprint(w, `{"error":false,"body":{"illustManga":{"data":[]}}}`)
		}
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"missing", "null", "id"} {
		result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: word})
		if result != nil || !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) {
			t.Errorf("%s result=%#v err=%v", word, result, err)
		}
	}
	result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "empty"})
	if err != nil || result == nil || result.Illusts == nil {
		t.Fatalf("explicit empty = %#v, %v", result, err)
	}
}

func TestAnonymousSearchUserCursorFollowsArtworkBatchNotDeduplicatedUsers(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("p") {
		case "1":
			fmt.Fprint(w, `{"error":false,"body":{"illustManga":{"total":4,"data":[{"id":"1","userId":"10","userName":"a"},{"id":"2","userId":"10","userName":"a"},{"id":"3","userId":"20","userName":"b"}]}}}`)
		case "2":
			fmt.Fprint(w, `{"error":false,"body":{"illustManga":{"total":4,"data":[{"id":"4","userId":"30","userName":"c"}]}}}`)
		}
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.SearchUser(context.Background(), pixiv.SearchUserRequest{Word: "artist"})
	if err != nil || len(first.UserPreviews) != 2 || first.NextCursor == "" {
		t.Fatalf("first = %#v, %v", first, err)
	}
	second, err := client.SearchUser(context.Background(), pixiv.SearchUserRequest{Word: "artist", Cursor: first.NextCursor})
	if err != nil || len(second.UserPreviews) != 1 || second.UserPreviews[0].User.ID != 30 || second.NextCursor != "" {
		t.Fatalf("second = %#v, %v", second, err)
	}
}

func TestAnonymousRankingUsesWebCursor(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ranking.php" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Web authorization must be empty")
		}
		if r.URL.Query().Get("p") == "1" {
			fmt.Fprint(w, `{"rank_total":2,"contents":[{"illust_id":1,"user_id":10}]}`)
		} else {
			fmt.Fprint(w, `{"rank_total":2,"contents":[{"illust_id":2,"user_id":20}]}`)
		}
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.IllustRanking(context.Background(), pixiv.IllustRankingRequest{})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := client.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Cursor: first.NextCursor})
	if err != nil || len(second.Illusts) != 1 || second.Illusts[0].ID != 2 || second.NextCursor != "" {
		t.Fatalf("second=%#v err=%v", second, err)
	}
}

func TestNonWhitelistedAnonymousOperationsAreUnauthorizedBeforeNetwork(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); http.Error(w, "unexpected", 500) }))
	defer server.Close()
	for _, fallback := range []bool{false, true} {
		client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, WebFallbackEnabled: fallback})
		if err != nil {
			t.Fatal(err)
		}
		calls := []func() error{
			func() error {
				_, err := client.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
				return err
			},
			func() error {
				_, err := client.FollowingIllusts(context.Background(), pixiv.FollowingIllustsRequest{})
				return err
			},
			func() error {
				_, err := client.UserDetail(context.Background(), pixiv.UserDetailRequest{UserID: 1})
				return err
			},
			func() error {
				_, err := client.UserArtworks(context.Background(), pixiv.UserArtworksRequest{UserID: 1})
				return err
			},
			func() error {
				_, err := client.UserBookmarks(context.Background(), pixiv.UserBookmarksRequest{UserID: 1})
				return err
			},
			func() error {
				_, err := client.UserFollowing(context.Background(), pixiv.UserFollowingRequest{UserID: 1})
				return err
			},
		}
		for _, call := range calls {
			if err := call(); !errors.Is(err, pixiv.ErrUnauthorized) {
				t.Errorf("fallback=%v error=%v", fallback, err)
			}
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("network requests = %d", requests.Load())
	}
}

func TestAuthenticatedAppFailureNeverFallsBackToWeb(t *testing.T) {
	t.Parallel()
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "secret-body", http.StatusBadGateway) }))
	defer app.Close()
	var webRequests atomic.Int32
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRequests.Add(1)
		fmt.Fprint(w, `{"error":false,"body":{"illustManga":{"data":[]}}}`)
	}))
	defer web.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: app.Client(), AppAPIBaseURL: app.URL, WebAPIBaseURL: web.URL, AccessToken: "token", WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku"})
	if result != nil || webRequests.Load() != 0 {
		t.Fatalf("result=%#v web_requests=%d", result, webRequests.Load())
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Backend != pixiv.BackendAppAPI || typed.Operation != pixiv.OperationSearchIllust || typed.UpstreamStatus != http.StatusBadGateway {
		t.Fatalf("error=%#v", typed)
	}
	if strings.Contains(fmt.Sprint(err), "secret-body") {
		t.Fatalf("error leaked body: %v", err)
	}
}

func TestInvalidListEnumsFailBeforeNetwork(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); fmt.Fprint(w, `{"illusts":[]}`) }))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	calls := []func() error{
		func() error {
			_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "x", Target: "bad"})
			return err
		},
		func() error {
			_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "x", Sort: "bad"})
			return err
		},
		func() error {
			_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "x", Duration: "bad"})
			return err
		},
		func() error {
			_, err := client.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Mode: "bad"})
			return err
		},
		func() error {
			_, err := client.FollowingIllusts(context.Background(), pixiv.FollowingIllustsRequest{Restrict: "bad"})
			return err
		},
		func() error {
			_, err := client.UserArtworks(context.Background(), pixiv.UserArtworksRequest{UserID: 1, Type: "bad"})
			return err
		},
		func() error {
			_, err := client.UserBookmarks(context.Background(), pixiv.UserBookmarksRequest{UserID: 1, Restrict: "bad"})
			return err
		},
		func() error {
			_, err := client.UserFollowing(context.Background(), pixiv.UserFollowingRequest{UserID: 1, Restrict: "bad"})
			return err
		},
	}
	for _, call := range calls {
		if err := call(); !errors.Is(err, pixiv.ErrInvalidArgument) {
			t.Errorf("error = %v", err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("network requests = %d", requests.Load())
	}
}

func TestUserBookmarksCursorCarriesOnlyBoundMaxBookmarkID(t *testing.T) {
	t.Parallel()
	const secret = "bookmark-next-secret"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Query().Get("max_bookmark_id") == "" {
			fmt.Fprintf(w, `{"illusts":[{"id":8}],"next_url":"/v1/user/bookmarks/illust?max_bookmark_id=777&proxy_password=%s"}`, secret)
			return
		}
		if r.URL.Query().Get("max_bookmark_id") != "777" {
			t.Errorf("max_bookmark_id = %q", r.URL.Query().Get("max_bookmark_id"))
		}
		fmt.Fprint(w, `{"illusts":[]}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	request := pixiv.UserBookmarksRequest{UserID: 42, Restrict: pixiv.RestrictPrivate, Tag: "cat"}
	first, err := client.UserBookmarks(context.Background(), request)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(string(first.NextCursor))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{secret, "cat", "private", "proxy_password", "bookmarks/illust"} {
		if strings.Contains(string(decoded), forbidden) {
			t.Fatalf("cursor contains %q: %s", forbidden, decoded)
		}
	}
	changed := request
	changed.Tag = "dog"
	changed.Cursor = first.NextCursor
	if _, err := client.UserBookmarks(context.Background(), changed); !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("changed tag error=%v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("changed query used network")
	}
	request.Cursor = first.NextCursor
	second, err := client.UserBookmarks(context.Background(), request)
	if err != nil || len(second.Illusts) != 0 || second.NextCursor != "" || requests.Load() != 2 {
		t.Fatalf("second=%#v err=%v requests=%d", second, err, requests.Load())
	}
}

func TestListOperationErrorsExposeStableOperationAndUserMetadata(t *testing.T) {
	t.Parallel()
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return testHTTPResponse(r, http.StatusInternalServerError, "metadata-secret"), nil
		})},
		AppAPIBaseURL: "https://app.invalid",
		AccessToken:   "token",
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		operation pixiv.Operation
		userID    int64
		call      func() error
	}{
		{pixiv.OperationSearchIllust, 0, func() error {
			_, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "x"})
			return err
		}},
		{pixiv.OperationIllustRanking, 0, func() error {
			_, err := client.IllustRanking(context.Background(), pixiv.IllustRankingRequest{})
			return err
		}},
		{pixiv.OperationIllustRecommended, 0, func() error {
			_, err := client.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
			return err
		}},
		{pixiv.OperationFollowingIllusts, 0, func() error {
			_, err := client.FollowingIllusts(context.Background(), pixiv.FollowingIllustsRequest{})
			return err
		}},
		{pixiv.OperationSearchUser, 0, func() error {
			_, err := client.SearchUser(context.Background(), pixiv.SearchUserRequest{Word: "x"})
			return err
		}},
		{pixiv.OperationUserDetail, 42, func() error {
			_, err := client.UserDetail(context.Background(), pixiv.UserDetailRequest{UserID: 42})
			return err
		}},
		{pixiv.OperationUserArtworks, 42, func() error {
			_, err := client.UserArtworks(context.Background(), pixiv.UserArtworksRequest{UserID: 42})
			return err
		}},
		{pixiv.OperationUserBookmarks, 42, func() error {
			_, err := client.UserBookmarks(context.Background(), pixiv.UserBookmarksRequest{UserID: 42})
			return err
		}},
		{pixiv.OperationUserFollowing, 42, func() error {
			_, err := client.UserFollowing(context.Background(), pixiv.UserFollowingRequest{UserID: 42})
			return err
		}},
	}
	for _, test := range tests {
		err := test.call()
		var typed *pixiv.Error
		if !errors.As(err, &typed) || typed.Operation != test.operation || typed.Backend != pixiv.BackendAppAPI || typed.UserID != test.userID || typed.IllustID != 0 || typed.UpstreamStatus != 500 {
			t.Errorf("%s error = %#v", test.operation, typed)
		}
		if strings.Contains(fmt.Sprint(err), "metadata-secret") || strings.Contains(fmt.Sprint(errors.Unwrap(err)), "metadata-secret") {
			t.Errorf("%s leaked response body", test.operation)
		}
	}
}

func TestUserListContextCancellationPreservesCauseWithoutPartialResult(t *testing.T) {
	t.Parallel()
	client, err := pixiv.NewClient(pixiv.Options{
		HTTPClient:    &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, context.Canceled })},
		AppAPIBaseURL: "https://app.invalid",
		AccessToken:   "token",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.UserFollowing(context.Background(), pixiv.UserFollowingRequest{UserID: 42})
	if result != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, pixiv.ErrUpstreamUnavailable) {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Operation != pixiv.OperationUserFollowing || typed.Backend != pixiv.BackendAppAPI || typed.UserID != 42 || typed.Retryable {
		t.Fatalf("metadata=%#v", typed)
	}
}

func TestUserWireRequiresUserAndPreviewFields(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := client.UserDetail(context.Background(), pixiv.UserDetailRequest{UserID: 42}); result != nil || !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) {
		t.Errorf("detail=%#v err=%v", result, err)
	}
	if result, err := client.SearchUser(context.Background(), pixiv.SearchUserRequest{Word: "x"}); result != nil || !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) {
		t.Errorf("search=%#v err=%v", result, err)
	}
}

func TestSearchIllustAnonymousWebCursorUsesNextWebBatch(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Web Authorization = %q, want empty", got)
		}
		switch r.URL.Query().Get("p") {
		case "1":
			fmt.Fprint(w, `{"error":false,"body":{"illustManga":{"total":2,"data":[{"id":"1","title":"one","userId":"10"}]}}}`)
		case "2":
			fmt.Fprint(w, `{"error":false,"body":{"illustManga":{"total":2,"data":[{"id":"2","title":"two","userId":"20"}]}}}`)
		default:
			t.Errorf("unexpected page %q", r.URL.Query().Get("p"))
		}
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}

	first, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku"})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Illusts) != 1 || first.NextCursor == "" {
		t.Fatalf("first = %#v", first)
	}
	second, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Illusts) != 1 || second.Illusts[0].ID != 2 || second.NextCursor != "" || requests.Load() != 2 {
		t.Fatalf("second = %#v; requests=%d", second, requests.Load())
	}
}
