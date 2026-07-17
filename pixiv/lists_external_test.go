package pixiv_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func largestSafeWebOffset(pageSize int) int {
	maxInt := int(^uint(0) >> 1)
	return (maxInt/pageSize)*pageSize - 1
}

func TestSearchIllustRejectsWebCursorWhoseNextPageBoundaryOverflowsBeforeNetwork(t *testing.T) {
	t.Parallel()
	const secret = "web-cursor-overflow-secret"
	maxInt := int(^uint(0) >> 1)
	var appRequests atomic.Int32
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appRequests.Add(1)
		switch r.URL.Query().Get("offset") {
		case "":
			fmt.Fprintf(w, `{"illusts":[],"next_url":"/v1/search/illust?offset=%s&proxy_password=%s"}`, strconv.Itoa(maxInt), secret)
		case strconv.Itoa(maxInt):
			fmt.Fprint(w, `{"illusts":[],"next_url":null}`)
		default:
			t.Errorf("App offset = %q", r.URL.Query().Get("offset"))
		}
	}))
	defer app.Close()

	authenticated, err := pixiv.NewClient(pixiv.Options{HTTPClient: app.Client(), AppAPIBaseURL: app.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := authenticated.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku"})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("App cursor = %q, error = %v", first.NextCursor, err)
	}
	appResult, err := authenticated.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: first.NextCursor})
	if err != nil || appResult == nil || appResult.NextCursor != "" || appRequests.Load() != 2 {
		t.Fatalf("App continuation = %#v, error = %v, requests = %d", appResult, err, appRequests.Load())
	}

	var webRequests atomic.Int32
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRequests.Add(1)
		fmt.Fprint(w, `{"error":false,"body":{"illustManga":{"data":[]}}}`)
	}))
	defer web.Close()
	anonymous, err := pixiv.NewClient(pixiv.Options{HTTPClient: web.Client(), WebAPIBaseURL: web.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := anonymous.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: first.NextCursor})
	if result != nil || !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationSearchIllust || typed.Backend != "" || typed.Retryable || typed.UpstreamStatus != 0 || typed.IllustID != 0 || typed.UserID != 0 {
		t.Fatalf("typed error = %#v", typed)
	}
	if webRequests.Load() != 0 {
		t.Fatalf("Web requests = %d, want 0", webRequests.Load())
	}
	for _, exposed := range []string{fmt.Sprint(err), fmt.Sprint(errors.Unwrap(err))} {
		if strings.Contains(exposed, string(first.NextCursor)) || strings.Contains(exposed, secret) || strings.Contains(exposed, "proxy_password") {
			t.Fatalf("error leaked cursor source: %q", exposed)
		}
	}
}

func TestIllustRankingRejectsWebCursorWhoseNextPageBoundaryOverflowsBeforeNetwork(t *testing.T) {
	t.Parallel()
	maxInt := int(^uint(0) >> 1)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"illusts":[],"next_url":"/v1/illust/ranking?offset=%s"}`, strconv.Itoa(maxInt))
	}))
	defer app.Close()
	authenticated, err := pixiv.NewClient(pixiv.Options{HTTPClient: app.Client(), AppAPIBaseURL: app.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := authenticated.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Mode: pixiv.RankingModeWeek})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("App cursor = %q, error = %v", first.NextCursor, err)
	}

	var webRequests atomic.Int32
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRequests.Add(1)
		fmt.Fprint(w, `{"rank_total":0,"contents":[]}`)
	}))
	defer web.Close()
	anonymous, err := pixiv.NewClient(pixiv.Options{HTTPClient: web.Client(), WebAPIBaseURL: web.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := anonymous.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Mode: pixiv.RankingModeWeek, Cursor: first.NextCursor})
	if result != nil || !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Operation != pixiv.OperationIllustRanking || typed.Backend != "" || typed.Retryable || typed.UpstreamStatus != 0 || typed.IllustID != 0 || typed.UserID != 0 {
		t.Fatalf("typed error = %#v", typed)
	}
	if webRequests.Load() != 0 {
		t.Fatalf("Web requests = %d, want 0", webRequests.Load())
	}
}

func TestSearchUserRejectsWebCursorWhoseNextPageBoundaryOverflowsWithSearchUserMetadata(t *testing.T) {
	t.Parallel()
	maxInt := int(^uint(0) >> 1)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"user_previews":[],"next_url":"/v1/search/user?offset=%s"}`, strconv.Itoa(maxInt))
	}))
	defer app.Close()
	authenticated, err := pixiv.NewClient(pixiv.Options{HTTPClient: app.Client(), AppAPIBaseURL: app.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := authenticated.SearchUser(context.Background(), pixiv.SearchUserRequest{Word: "artist"})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("App cursor = %q, error = %v", first.NextCursor, err)
	}

	var webRequests atomic.Int32
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRequests.Add(1)
		fmt.Fprint(w, `{"error":false,"body":{"illustManga":{"data":[]}}}`)
	}))
	defer web.Close()
	anonymous, err := pixiv.NewClient(pixiv.Options{HTTPClient: web.Client(), WebAPIBaseURL: web.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := anonymous.SearchUser(context.Background(), pixiv.SearchUserRequest{Word: "artist", Cursor: first.NextCursor})
	if result != nil || !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Operation != pixiv.OperationSearchUser || typed.Backend != "" || typed.Retryable || typed.UpstreamStatus != 0 || typed.IllustID != 0 || typed.UserID != 0 {
		t.Fatalf("typed error = %#v", typed)
	}
	if webRequests.Load() != 0 {
		t.Fatalf("Web requests = %d, want 0", webRequests.Load())
	}
}

func TestSearchIllustLargestSafeWebCursorComparesTotalWithoutOverflow(t *testing.T) {
	t.Parallel()
	const pageSize = 60
	offset := largestSafeWebOffset(pageSize)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"illusts":[],"next_url":"/v1/search/illust?offset=%d"}`, offset)
	}))
	defer app.Close()
	authenticated, err := pixiv.NewClient(pixiv.Options{HTTPClient: app.Client(), AppAPIBaseURL: app.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := authenticated.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "boundary"})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("App cursor = %q, error = %v", first.NextCursor, err)
	}

	var webRequests atomic.Int32
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRequests.Add(1)
		items := make([]map[string]any, 68)
		for index := range items {
			items[index] = map[string]any{"id": index + 1, "userId": index + 101}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": false,
			"body": map[string]any{"illustManga": map[string]any{
				"total": int(^uint(0) >> 1),
				"data":  items,
			}},
		})
	}))
	defer web.Close()
	anonymous, err := pixiv.NewClient(pixiv.Options{HTTPClient: web.Client(), WebAPIBaseURL: web.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := anonymous.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "boundary", Cursor: first.NextCursor})
	if err != nil || result == nil || len(result.Illusts) != 9 || result.NextCursor != "" || webRequests.Load() != 1 {
		t.Fatalf("result = %#v, error = %v, requests = %d", result, err, webRequests.Load())
	}
}

func TestLargestSafeWebCursorRequestsOneEmptyBatchAcrossOperations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		pageSize int
		appBody  func(int) string
		appCall  func(*pixiv.Client) (pixiv.Cursor, error)
		webPath  string
		webBody  string
		webCall  func(*pixiv.Client, pixiv.Cursor) (int, pixiv.Cursor, error)
	}{
		{
			name:     "search illust",
			pageSize: 60,
			appBody: func(offset int) string {
				return fmt.Sprintf(`{"illusts":[],"next_url":"/v1/search/illust?offset=%d"}`, offset)
			},
			appCall: func(client *pixiv.Client) (pixiv.Cursor, error) {
				result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "safe"})
				if err != nil {
					return "", err
				}
				return result.NextCursor, nil
			},
			webPath: "/ajax/search/artworks/safe",
			webBody: `{"error":false,"body":{"illustManga":{"data":[]}}}`,
			webCall: func(client *pixiv.Client, cursor pixiv.Cursor) (int, pixiv.Cursor, error) {
				result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "safe", Cursor: cursor})
				if err != nil {
					return 0, "", err
				}
				return len(result.Illusts), result.NextCursor, nil
			},
		},
		{
			name:     "illust ranking",
			pageSize: 50,
			appBody: func(offset int) string {
				return fmt.Sprintf(`{"illusts":[],"next_url":"/v1/illust/ranking?offset=%d"}`, offset)
			},
			appCall: func(client *pixiv.Client) (pixiv.Cursor, error) {
				result, err := client.IllustRanking(context.Background(), pixiv.IllustRankingRequest{})
				if err != nil {
					return "", err
				}
				return result.NextCursor, nil
			},
			webPath: "/ranking.php",
			webBody: `{"rank_total":0,"contents":[]}`,
			webCall: func(client *pixiv.Client, cursor pixiv.Cursor) (int, pixiv.Cursor, error) {
				result, err := client.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Cursor: cursor})
				if err != nil {
					return 0, "", err
				}
				return len(result.Illusts), result.NextCursor, nil
			},
		},
		{
			name:     "search user",
			pageSize: 60,
			appBody: func(offset int) string {
				return fmt.Sprintf(`{"user_previews":[],"next_url":"/v1/search/user?offset=%d"}`, offset)
			},
			appCall: func(client *pixiv.Client) (pixiv.Cursor, error) {
				result, err := client.SearchUser(context.Background(), pixiv.SearchUserRequest{Word: "safe"})
				if err != nil {
					return "", err
				}
				return result.NextCursor, nil
			},
			webPath: "/ajax/search/artworks/safe",
			webBody: `{"error":false,"body":{"illustManga":{"data":[]}}}`,
			webCall: func(client *pixiv.Client, cursor pixiv.Cursor) (int, pixiv.Cursor, error) {
				result, err := client.SearchUser(context.Background(), pixiv.SearchUserRequest{Word: "safe", Cursor: cursor})
				if err != nil {
					return 0, "", err
				}
				return len(result.UserPreviews), result.NextCursor, nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			offset := largestSafeWebOffset(test.pageSize)
			app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, test.appBody(offset))
			}))
			defer app.Close()
			authenticated, err := pixiv.NewClient(pixiv.Options{HTTPClient: app.Client(), AppAPIBaseURL: app.URL, AccessToken: "token"})
			if err != nil {
				t.Fatal(err)
			}
			cursor, err := test.appCall(authenticated)
			if err != nil || cursor == "" {
				t.Fatalf("App cursor = %q, error = %v", cursor, err)
			}

			var webRequests atomic.Int32
			web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				webRequests.Add(1)
				if r.URL.Path != test.webPath {
					t.Errorf("Web path = %q, want %q", r.URL.Path, test.webPath)
				}
				if r.URL.Query().Get("p") != strconv.Itoa(offset/test.pageSize+1) {
					t.Errorf("Web page = %q", r.URL.Query().Get("p"))
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Errorf("Web request carried SDK credentials")
				}
				fmt.Fprint(w, test.webBody)
			}))
			defer web.Close()
			anonymous, err := pixiv.NewClient(pixiv.Options{HTTPClient: web.Client(), WebAPIBaseURL: web.URL, WebFallbackEnabled: true})
			if err != nil {
				t.Fatal(err)
			}
			count, next, err := test.webCall(anonymous, cursor)
			if err != nil || count != 0 || next != "" || webRequests.Load() != 1 {
				t.Fatalf("count = %d, next = %q, error = %v, requests = %d", count, next, err, webRequests.Load())
			}
		})
	}
}

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

func assertOpaqueCursorDoesNotContain(t *testing.T, cursor pixiv.Cursor, forbidden ...string) {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(string(cursor))
	if err != nil {
		t.Fatalf("cursor is not opaque encoding: %v", err)
	}
	for _, exposed := range []string{string(cursor), string(decoded)} {
		for _, value := range forbidden {
			if strings.Contains(exposed, value) {
				t.Fatalf("cursor leaks %q: %q", value, exposed)
			}
		}
	}
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
			fmt.Fprint(w, `{"user":{"id":42,"name":"detail"},"profile":{},"profile_publicity":{},"workspace":{}}`)
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

func TestUserDetailReturnsCompleteStableProfileFromOneAppRequest(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/v1/user/detail" || r.URL.Query().Get("user_id") != "42" {
			t.Fatalf("request path=%q query=%q", r.URL.Path, r.URL.RawQuery)
		}
		fmt.Fprint(w, `{
			"user":{"id":42,"name":"alice","account":"alice_account","comment":"hello","is_followed":true,"profile_image_urls":{"medium":"https://img.example/profile.jpg"}},
			"profile":{"webpage":"https://alice.example","gender":"female","birth":"2000-01-02","birth_day":"01-02","birth_year":2000,"region":"Tokyo","address_id":13,"country_code":"JP","job":"illustrator","job_id":9,"total_follow_users":10,"total_mypixiv_users":11,"total_illusts":12,"total_manga":13,"total_novels":14,"total_illust_bookmarks_public":15,"total_illust_series":16,"total_novel_series":17,"background_image_url":"https://img.example/background.jpg","twitter_account":"alice","twitter_url":"https://x.example/alice","pawoo_url":"https://pawoo.example/@alice","is_premium":true,"is_using_custom_profile_image":true},
			"profile_publicity":{"gender":"public","region":"private","birth_day":"public","birth_year":"private","job":"public","pawoo":true},
			"workspace":{"pc":"PC","monitor":"Monitor","tool":"Tool","scanner":"Scanner","tablet":"Tablet","mouse":"Mouse","printer":"Printer","desktop":"Desktop","music":"Music","desk":"Desk","chair":"Chair","comment":"Workspace","workspace_image_url":"https://img.example/workspace.jpg"}
		}`)
	}))
	defer server.Close()

	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := client.UserDetail(context.Background(), pixiv.UserDetailRequest{UserID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if detail.User.ID != 42 || detail.User.Name != "alice" || detail.User.ProfileImageURLs.Medium == nil || *detail.User.ProfileImageURLs.Medium != "https://img.example/profile.jpg" {
		t.Fatalf("user=%#v", detail.User)
	}
	if detail.Profile.Webpage == nil || *detail.Profile.Webpage != "https://alice.example" || detail.Profile.TotalNovelSeries != 17 || detail.Profile.BackgroundImageURL == nil || *detail.Profile.BackgroundImageURL != "https://img.example/background.jpg" || detail.Profile.TwitterURL == nil || detail.Profile.PawooURL == nil || !detail.Profile.IsPremium || !detail.Profile.IsUsingCustomProfileImage {
		t.Fatalf("profile=%#v", detail.Profile)
	}
	if !detail.ProfilePublicity.Gender || detail.ProfilePublicity.Region || !detail.ProfilePublicity.BirthDay || detail.ProfilePublicity.BirthYear || !detail.ProfilePublicity.Job || !detail.ProfilePublicity.Pawoo {
		t.Fatalf("profile_publicity=%#v", detail.ProfilePublicity)
	}
	if detail.Workspace.PC != "PC" || detail.Workspace.Comment != "Workspace" || detail.Workspace.WorkspaceImageURL == nil || *detail.Workspace.WorkspaceImageURL != "https://img.example/workspace.jpg" {
		t.Fatalf("workspace=%#v", detail.Workspace)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests=%d, want 1", requests.Load())
	}
}

func TestUserDetailRejectsMalformedRequiredEnvelopesWithTypedSafeError(t *testing.T) {
	t.Parallel()
	const secret = "user-detail-response-secret"
	valid := `{"user":{"id":42},"profile":{},"profile_publicity":{},"workspace":{}}`
	tests := []struct {
		name string
		body string
	}{
		{name: "user missing", body: `{"profile":{},"profile_publicity":{},"workspace":{}}`},
		{name: "user null", body: `{"user":null,"profile":{},"profile_publicity":{},"workspace":{}}`},
		{name: "user non object", body: `{"user":"` + secret + `","profile":{},"profile_publicity":{},"workspace":{}}`},
		{name: "profile missing", body: `{"user":{"id":42},"profile_publicity":{},"workspace":{}}`},
		{name: "profile null", body: `{"user":{"id":42},"profile":null,"profile_publicity":{},"workspace":{}}`},
		{name: "profile non object", body: `{"user":{"id":42},"profile":[],"profile_publicity":{},"workspace":{}}`},
		{name: "profile publicity missing", body: `{"user":{"id":42},"profile":{},"workspace":{}}`},
		{name: "profile publicity null", body: `{"user":{"id":42},"profile":{},"profile_publicity":null,"workspace":{}}`},
		{name: "profile publicity non object", body: `{"user":{"id":42},"profile":{},"profile_publicity":false,"workspace":{}}`},
		{name: "workspace missing", body: `{"user":{"id":42},"profile":{},"profile_publicity":{}}`},
		{name: "workspace null", body: `{"user":{"id":42},"profile":{},"profile_publicity":{},"workspace":null}`},
		{name: "workspace non object", body: `{"user":{"id":42},"profile":{},"profile_publicity":{},"workspace":1}`},
		{name: "user id zero", body: strings.Replace(valid, `"id":42`, `"id":0`, 1)},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, test.body) }))
			defer server.Close()
			client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.UserDetail(context.Background(), pixiv.UserDetailRequest{UserID: 42})
			if result != nil || !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			var typed *pixiv.Error
			if !errors.As(err, &typed) || typed.Operation != pixiv.OperationUserDetail || typed.Backend != pixiv.BackendAppAPI || typed.UserID != 42 || typed.Retryable || typed.UpstreamStatus != 0 {
				t.Fatalf("typed=%#v", typed)
			}
			for _, exposed := range []string{err.Error(), fmt.Sprint(errors.Unwrap(err))} {
				if strings.Contains(exposed, secret) || strings.Contains(exposed, server.URL) || strings.Contains(exposed, "token") {
					t.Fatalf("unsafe error=%q", exposed)
				}
			}
		})
	}
}

func TestUserDetailNormalizesOptionalURLsAndUnknownFieldsToStableZeroValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		profileImageURL string
		profileURLs     string
		workspaceURL    string
	}{
		{name: "absent", profileImageURL: `{}`, profileURLs: `{}`, workspaceURL: `{}`},
		{name: "null", profileImageURL: `{"medium":null}`, profileURLs: `{"webpage":null,"background_image_url":null,"twitter_url":null,"pawoo_url":null}`, workspaceURL: `{"workspace_image_url":null}`},
		{name: "empty", profileImageURL: `{"medium":""}`, profileURLs: `{"webpage":"","background_image_url":"","twitter_url":"","pawoo_url":""}`, workspaceURL: `{"workspace_image_url":""}`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintf(w, `{"user":{"id":42,"profile_image_urls":%s,"private_text":"hidden"},"profile":%s,"profile_publicity":{},"workspace":%s,"unknown_count":999}`, test.profileImageURL, test.profileURLs, test.workspaceURL)
			}))
			defer server.Close()
			client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
			if err != nil {
				t.Fatal(err)
			}
			detail, err := client.UserDetail(context.Background(), pixiv.UserDetailRequest{UserID: 42})
			if err != nil {
				t.Fatal(err)
			}
			if detail.User.ProfileImageURLs.Medium != nil || detail.Profile.Webpage != nil || detail.Profile.BackgroundImageURL != nil || detail.Profile.TwitterURL != nil || detail.Profile.PawooURL != nil || detail.Workspace.WorkspaceImageURL != nil {
				t.Fatalf("optional urls were not normalized: %#v", detail)
			}
			if detail.User.Name != "" || detail.User.Comment != "" || detail.Profile.TotalFollowUsers != 0 || detail.Profile.TotalNovelSeries != 0 || detail.Profile.IsPremium || detail.Profile.IsUsingCustomProfileImage || detail.ProfilePublicity.Gender || detail.Workspace.Comment != "" {
				t.Fatalf("hidden or unknown data escaped stable zero values: %#v", detail)
			}
		})
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

func TestSearchIllustHighResolutionUsesAppServerBounds(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search/illust" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("width_min") != "3000" || query.Get("height_min") != "3000" {
			t.Fatalf("query = %v", query)
		}
		fmt.Fprint(w, `{"illusts":[]}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{
		Word: "miku",
		Filters: pixiv.SearchIllustFilters{
			Resolution: pixiv.SearchResolutionHigh,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearchIllustOptionsUsesAuthenticatedApp(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search/options" || r.URL.Query().Get("word") != "miku" {
			t.Fatalf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"illust":{"tool":{"options":["CLIP STUDIO PAINT","Photoshop"]}}}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SearchIllustOptions(context.Background(), pixiv.SearchIllustOptionsRequest{Word: " miku "})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 2 || result.Tools[0] != "CLIP STUDIO PAINT" || result.Tools[1] != "Photoshop" {
		t.Fatalf("tools = %#v", result.Tools)
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

func TestListDecoderHonorsFinalDuplicateJSONValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		body      string
		web       bool
		wantError bool
	}{
		{name: "App final null is malformed", body: `{"illusts":[],"illusts":null}`, wantError: true},
		{name: "App final array is valid", body: `{"illusts":null,"illusts":[]}`},
		{name: "Web final null is malformed", body: `{"error":false,"body":{"illustManga":{"data":[],"data":null}}}`, web: true, wantError: true},
		{name: "Web final array is valid", body: `{"error":false,"body":{"illustManga":{"data":null,"data":[]}}}`, web: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, test.body) }))
			defer server.Close()
			options := pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"}
			if test.web {
				options = pixiv.Options{HTTPClient: server.Client(), WebAPIBaseURL: server.URL, WebFallbackEnabled: true}
			}
			client, err := pixiv.NewClient(options)
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku"})
			if test.wantError {
				if result != nil || !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) {
					t.Fatalf("result=%#v err=%v", result, err)
				}
				return
			}
			if err != nil || result == nil || result.Illusts == nil || len(result.Illusts) != 0 {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
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

func TestAnonymousSearchUserRejectsArtworkWithoutUserID(t *testing.T) {
	t.Parallel()
	const secret = "missing-user-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"error":false,"body":{"illustManga":{"data":[{"id":"1","userName":"%s"}]}}}`, secret)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SearchUser(context.Background(), pixiv.SearchUserRequest{Word: "artist"})
	if result != nil || !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeMalformedUpstreamResponse || typed.Backend != pixiv.BackendWebAPI || typed.Operation != pixiv.OperationSearchUser || typed.UserID != 0 || typed.IllustID != 0 {
		t.Fatalf("metadata=%#v", typed)
	}
	if strings.Contains(fmt.Sprint(err), secret) || strings.Contains(fmt.Sprint(errors.Unwrap(err)), secret) {
		t.Fatalf("error leaked source data: %v", err)
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

func TestExtendedRankingModesUseAppAndWebWireValuesAndBindCursor(t *testing.T) {
	t.Parallel()
	modes := []struct {
		mode pixiv.RankingMode
		web  string
	}{
		{pixiv.RankingModeDayMale, "male"},
		{pixiv.RankingModeDayFemale, "female"},
		{pixiv.RankingModeWeekOriginal, "original"},
		{pixiv.RankingModeWeekRookie, "rookie"},
	}
	for index, test := range modes {
		t.Run(string(test.mode), func(t *testing.T) {
			var appRequests atomic.Int32
			app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				appRequests.Add(1)
				if got := r.URL.Query().Get("mode"); got != string(test.mode) {
					t.Errorf("App mode=%q", got)
				}
				fmt.Fprint(w, `{"illusts":[{"id":1}],"next_url":"/v1/illust/ranking?offset=30"}`)
			}))
			defer app.Close()
			appClient, err := pixiv.NewClient(pixiv.Options{HTTPClient: app.Client(), AppAPIBaseURL: app.URL, AccessToken: "token"})
			if err != nil {
				t.Fatal(err)
			}
			appResult, err := appClient.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Mode: test.mode})
			if err != nil || appResult.NextCursor == "" {
				t.Fatalf("App result=%#v err=%v", appResult, err)
			}
			other := modes[(index+1)%len(modes)].mode
			if _, err := appClient.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Mode: other, Cursor: appResult.NextCursor}); !errors.Is(err, pixiv.ErrInvalidArgument) {
				t.Fatalf("App cursor mismatch error=%v", err)
			}
			if appRequests.Load() != 1 {
				t.Fatalf("App mismatch used network")
			}

			var webRequests atomic.Int32
			web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				webRequests.Add(1)
				if got := r.URL.Query().Get("mode"); got != test.web {
					t.Errorf("Web mode=%q want=%q", got, test.web)
				}
				fmt.Fprint(w, `{"rank_total":2,"contents":[{"illust_id":1,"user_id":10}]}`)
			}))
			defer web.Close()
			webClient, err := pixiv.NewClient(pixiv.Options{HTTPClient: web.Client(), WebAPIBaseURL: web.URL, WebFallbackEnabled: true})
			if err != nil {
				t.Fatal(err)
			}
			webResult, err := webClient.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Mode: test.mode})
			if err != nil || webResult.NextCursor == "" {
				t.Fatalf("Web result=%#v err=%v", webResult, err)
			}
			if _, err := webClient.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Mode: other, Cursor: webResult.NextCursor}); !errors.Is(err, pixiv.ErrInvalidArgument) {
				t.Fatalf("Web cursor mismatch error=%v", err)
			}
			if webRequests.Load() != 1 {
				t.Fatalf("Web mismatch used network")
			}
		})
	}
}

func TestRankingDateRequiresCanonicalDateAndBindsCursor(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if got := r.URL.Query().Get("date"); got != "2025-01-02" {
			t.Errorf("App date=%q", got)
		}
		fmt.Fprint(w, `{"illusts":[{"id":1}],"next_url":"/v1/illust/ranking?offset=30"}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2025-1-2", "2025-02-30", " 2025-01-02 ", "20250102"} {
		result, err := client.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Date: date})
		if result != nil || !errors.Is(err, pixiv.ErrInvalidArgument) {
			t.Errorf("date=%q result=%#v err=%v", date, result, err)
		}
		var typed *pixiv.Error
		if !errors.As(err, &typed) || typed.Operation != pixiv.OperationIllustRanking || typed.Backend != "" {
			t.Errorf("date=%q metadata=%#v", date, typed)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid dates used network: %d", requests.Load())
	}
	first, err := client.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Date: "2025-01-02"})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	if _, err := client.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Date: "2025-01-03", Cursor: first.NextCursor}); !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("date mismatch error=%v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("date mismatch used network")
	}

	var webRequests atomic.Int32
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webRequests.Add(1)
		if got := r.URL.Query().Get("date"); got != "20250102" {
			t.Errorf("Web date=%q", got)
		}
		fmt.Fprint(w, `{"rank_total":2,"contents":[{"illust_id":1,"user_id":10}]}`)
	}))
	defer web.Close()
	webClient, err := pixiv.NewClient(pixiv.Options{HTTPClient: web.Client(), WebAPIBaseURL: web.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	webFirst, err := webClient.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Date: "2025-01-02"})
	if err != nil || webFirst.NextCursor == "" {
		t.Fatalf("Web first=%#v err=%v", webFirst, err)
	}
	if _, err := webClient.IllustRanking(context.Background(), pixiv.IllustRankingRequest{Date: "2025-01-03", Cursor: webFirst.NextCursor}); !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("Web date mismatch error=%v", err)
	}
	if webRequests.Load() != 1 {
		t.Fatalf("Web date mismatch used network")
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

func TestMangaRecommendedUsesIllustCatalogWithMangaContentType(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/illust/recommended" || r.URL.Query().Get("content_type") != "manga" {
			t.Fatalf("path=%q query=%q", r.URL.Path, r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"illusts":[{"id":101,"title":"manga","type":"manga"}],"next_url":null}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.MangaRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
	if err != nil || len(result.Illusts) != 1 || result.Illusts[0].ID != 101 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestRecommendationsAcceptAndReuseZeroOffsetCursors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		path        string
		contentType string
		itemsField  string
		call        func(*pixiv.Client, pixiv.Cursor) (pixiv.Cursor, error)
	}{
		{
			name: "illust", path: "/v1/illust/recommended", itemsField: "illusts",
			call: func(client *pixiv.Client, cursor pixiv.Cursor) (pixiv.Cursor, error) {
				result, err := client.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{Cursor: cursor})
				if err != nil || result == nil {
					return "", err
				}
				return result.NextCursor, nil
			},
		},
		{
			name: "manga", path: "/v1/illust/recommended", contentType: "manga", itemsField: "illusts",
			call: func(client *pixiv.Client, cursor pixiv.Cursor) (pixiv.Cursor, error) {
				result, err := client.MangaRecommended(context.Background(), pixiv.IllustRecommendedRequest{Cursor: cursor})
				if err != nil || result == nil {
					return "", err
				}
				return result.NextCursor, nil
			},
		},
		{
			name: "novel", path: "/v1/novel/recommended", itemsField: "novels",
			call: func(client *pixiv.Client, cursor pixiv.Cursor) (pixiv.Cursor, error) {
				result, err := client.NovelRecommended(context.Background(), pixiv.NovelRecommendedRequest{Cursor: cursor})
				if err != nil || result == nil {
					return "", err
				}
				return result.NextCursor, nil
			},
		},
		{
			name: "user", path: "/v1/user/recommended", itemsField: "user_previews",
			call: func(client *pixiv.Client, cursor pixiv.Cursor) (pixiv.Cursor, error) {
				result, err := client.UserRecommended(context.Background(), pixiv.UserRecommendedRequest{Cursor: cursor})
				if err != nil || result == nil {
					return "", err
				}
				return result.NextCursor, nil
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			const secret = "zero-offset-query-secret"
			rawNextURL := test.path + "?offset=0&pagination_secret=" + secret
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != test.path || r.URL.Query().Get("content_type") != test.contentType {
					t.Fatalf("path=%q query=%q", r.URL.Path, r.URL.RawQuery)
				}
				switch requests.Add(1) {
				case 1:
					if r.URL.Query().Has("offset") {
						t.Fatalf("initial query=%q", r.URL.RawQuery)
					}
					fmt.Fprintf(w, `{"%s":[],"next_url":%q}`, test.itemsField, rawNextURL)
				case 2:
					if offsets := r.URL.Query()["offset"]; len(offsets) != 1 || offsets[0] != "0" {
						t.Fatalf("continuation offsets=%q raw_query=%q", offsets, r.URL.RawQuery)
					}
					fmt.Fprintf(w, `{"%s":[],"next_url":null}`, test.itemsField)
				default:
					t.Fatalf("unexpected request count=%d", requests.Load())
				}
			}))
			defer server.Close()
			client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
			if err != nil {
				t.Fatal(err)
			}

			first, err := test.call(client, "")
			if err != nil || first == "" {
				t.Fatalf("first cursor=%q err=%v", first, err)
			}
			assertOpaqueCursorDoesNotContain(t, first, rawNextURL, secret, "pagination_secret", "next_url")
			second, err := test.call(client, first)
			if err != nil || second != "" || requests.Load() != 2 {
				t.Fatalf("second cursor=%q err=%v requests=%d", second, err, requests.Load())
			}
		})
	}
}

func TestNovelRecommendedMapsStableFieldsAndHidesUpstreamContinuation(t *testing.T) {
	t.Parallel()
	const secret = "novel-next-url-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/novel/recommended" || r.URL.Query().Get("content_type") != "" {
			t.Fatalf("path=%q query=%q", r.URL.Path, r.URL.RawQuery)
		}
		fmt.Fprintf(w, `{"novels":[{"id":201,"title":"novel","caption":"caption","user":{"id":8,"name":"author","account":"author_account"},"tags":[{"name":"tag","translated_name":"标签"}],"image_urls":{"medium":"https://img.example/novel.jpg"},"create_date":"2026-07-14T00:00:00+00:00","total_bookmarks":12,"total_view":34}],"next_url":"https://app-api.pixiv.net/v1/novel/recommended?offset=30&token=%s"}`, secret)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.NovelRecommended(context.Background(), pixiv.NovelRecommendedRequest{})
	if err != nil || len(result.Novels) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	novel := result.Novels[0]
	if novel.ID != 201 || novel.Caption != "caption" || novel.User.ID != 8 || len(novel.Tags) != 1 || novel.ImageURLs.Medium == "" || novel.CreateDate == "" || novel.TotalBookmarks != 12 || novel.TotalView != 34 || result.NextCursor == "" {
		t.Fatalf("novel=%#v cursor=%q", novel, result.NextCursor)
	}
	if strings.Contains(string(result.NextCursor), secret) || strings.Contains(string(result.NextCursor), "next_url") {
		t.Fatalf("cursor leaks upstream continuation: %q", result.NextCursor)
	}
}

func TestUserRecommendedMapsUserAndWorkPreviews(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/recommended" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		fmt.Fprint(w, `{"user_previews":[{"user":{"id":301,"name":"artist","account":"artist_account"},"illusts":[{"id":302,"title":"preview","user":{"id":301}}],"novels":[{"id":303,"title":"novel preview","caption":"caption","user":{"id":301},"tags":[],"image_urls":{},"create_date":"2026-07-14","total_bookmarks":2,"total_view":3}]}],"next_url":null,"ranking":99,"privacy_policy":{"secret":true}}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.UserRecommended(context.Background(), pixiv.UserRecommendedRequest{})
	if err != nil || len(result.UserPreviews) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	preview := result.UserPreviews[0]
	if preview.User.ID != 301 || preview.User.Name != "artist" || preview.User.Account != "artist_account" ||
		len(preview.Illusts) != 1 || preview.Illusts[0].ID != 302 || preview.Illusts[0].Title != "preview" ||
		len(preview.Novels) != 1 || preview.Novels[0].ID != 303 || preview.Novels[0].Caption != "caption" {
		t.Fatalf("preview=%#v", preview)
	}
}

func TestZeroOffsetContinuationRemainsRecommendationOnly(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"illusts":[],"next_url":"/v1/search/illust?offset=0"}`)
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "zero"})
	var typed *pixiv.Error
	if result != nil || !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) || !errors.As(err, &typed) || typed.Operation != pixiv.OperationSearchIllust || typed.Backend != pixiv.BackendAppAPI {
		t.Fatalf("result=%#v err=%#v typed=%#v", result, err, typed)
	}
}

func TestRecommendationsRejectInvalidOffsetContinuations(t *testing.T) {
	t.Parallel()
	operations := []struct {
		name     string
		path     string
		envelope func(string) string
		call     func(*pixiv.Client) error
		op       pixiv.Operation
	}{
		{
			name: "illust", path: "/v1/illust/recommended",
			envelope: func(next string) string { return fmt.Sprintf(`{"illusts":[],"next_url":%q}`, next) },
			call: func(client *pixiv.Client) error {
				_, err := client.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
				return err
			},
			op: pixiv.OperationIllustRecommended,
		},
		{
			name: "manga", path: "/v1/illust/recommended",
			envelope: func(next string) string { return fmt.Sprintf(`{"illusts":[],"next_url":%q}`, next) },
			call: func(client *pixiv.Client) error {
				_, err := client.MangaRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
				return err
			},
			op: pixiv.OperationMangaRecommended,
		},
		{
			name: "novel", path: "/v1/novel/recommended",
			envelope: func(next string) string { return fmt.Sprintf(`{"novels":[],"next_url":%q}`, next) },
			call: func(client *pixiv.Client) error {
				_, err := client.NovelRecommended(context.Background(), pixiv.NovelRecommendedRequest{})
				return err
			},
			op: pixiv.OperationNovelRecommended,
		},
		{
			name: "user", path: "/v1/user/recommended",
			envelope: func(next string) string { return fmt.Sprintf(`{"user_previews":[],"next_url":%q}`, next) },
			call: func(client *pixiv.Client) error {
				_, err := client.UserRecommended(context.Background(), pixiv.UserRecommendedRequest{})
				return err
			},
			op: pixiv.OperationUserRecommended,
		},
	}
	invalid := []struct {
		name string
		next string
	}{
		{name: "negative", next: "/recommended?offset=-1"},
		{name: "missing", next: "/recommended?token=secret"},
		{name: "duplicate", next: "/recommended?offset=0&offset=1"},
		{name: "non integer", next: "/recommended?offset=0.5"},
		{name: "overflow", next: "/recommended?offset=9223372036854775808"},
	}
	for _, operation := range operations {
		operation := operation
		for _, malformed := range invalid {
			malformed := malformed
			t.Run(operation.name+"/"+malformed.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != operation.path {
						t.Fatalf("path=%q", r.URL.Path)
					}
					fmt.Fprint(w, operation.envelope(malformed.next))
				}))
				defer server.Close()
				client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
				if err != nil {
					t.Fatal(err)
				}
				err = operation.call(client)
				var typed *pixiv.Error
				if !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) || !errors.As(err, &typed) || typed.Operation != operation.op || typed.Backend != pixiv.BackendAppAPI {
					t.Fatalf("err=%#v typed=%#v", err, typed)
				}
			})
		}
	}
}

func TestRecommendationCursorsAreKindSpecificAndNeverExposeNextURL(t *testing.T) {
	t.Parallel()
	const secret = "recommendation-next-url-secret"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/v1/illust/recommended":
			if r.URL.Query().Get("content_type") == "manga" {
				fmt.Fprintf(w, `{"illusts":[],"next_url":"/v1/illust/recommended?content_type=manga&offset=30&token=%s"}`, secret)
				return
			}
			if r.URL.Query().Get("content_type") != "" {
				t.Fatalf("illust content_type=%q", r.URL.Query().Get("content_type"))
			}
			fmt.Fprintf(w, `{"illusts":[],"next_url":"/v1/illust/recommended?offset=30&token=%s"}`, secret)
		case "/v1/novel/recommended":
			fmt.Fprintf(w, `{"novels":[],"next_url":"/v1/novel/recommended?offset=30&token=%s"}`, secret)
		case "/v1/user/recommended":
			fmt.Fprintf(w, `{"user_previews":[],"next_url":"/v1/user/recommended?offset=30&token=%s"}`, secret)
		default:
			t.Fatalf("unexpected path=%q", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	illust, err := client.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
	if err != nil {
		t.Fatal(err)
	}
	manga, err := client.MangaRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
	if err != nil {
		t.Fatal(err)
	}
	novel, err := client.NovelRecommended(context.Background(), pixiv.NovelRecommendedRequest{})
	if err != nil {
		t.Fatal(err)
	}
	users, err := client.UserRecommended(context.Background(), pixiv.UserRecommendedRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, cursor := range []pixiv.Cursor{illust.NextCursor, manga.NextCursor, novel.NextCursor, users.NextCursor} {
		if cursor == "" || strings.Contains(string(cursor), secret) || strings.Contains(string(cursor), "next_url") {
			t.Fatalf("unsafe cursor=%q", cursor)
		}
	}
	for _, call := range []func() error{
		func() error {
			_, err := client.MangaRecommended(context.Background(), pixiv.IllustRecommendedRequest{Cursor: illust.NextCursor})
			return err
		},
		func() error {
			_, err := client.NovelRecommended(context.Background(), pixiv.NovelRecommendedRequest{Cursor: manga.NextCursor})
			return err
		},
		func() error {
			_, err := client.UserRecommended(context.Background(), pixiv.UserRecommendedRequest{Cursor: novel.NextCursor})
			return err
		},
		func() error {
			_, err := client.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{Cursor: users.NextCursor})
			return err
		},
	} {
		if err := call(); !errors.Is(err, pixiv.ErrInvalidArgument) {
			t.Fatalf("cross-kind cursor error=%v", err)
		}
	}
	if requests.Load() != 4 {
		t.Fatalf("cross-kind cursors unexpectedly used network: requests=%d", requests.Load())
	}
}

func TestNovelAndUserRecommendedRejectMalformedEnvelopesAndContinuation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		body string
		call func(*pixiv.Client) error
		op   pixiv.Operation
	}{
		{"novel envelope", "/v1/novel/recommended", `{}`, func(c *pixiv.Client) error {
			_, err := c.NovelRecommended(context.Background(), pixiv.NovelRecommendedRequest{})
			return err
		}, pixiv.OperationNovelRecommended},
		{"novel continuation", "/v1/novel/recommended", `{"novels":[],"next_url":"/v1/novel/recommended?offset=bad"}`, func(c *pixiv.Client) error {
			_, err := c.NovelRecommended(context.Background(), pixiv.NovelRecommendedRequest{})
			return err
		}, pixiv.OperationNovelRecommended},
		{"novel id", "/v1/novel/recommended", `{"novels":[{"id":0,"user":{"id":1}}],"next_url":null}`, func(c *pixiv.Client) error {
			_, err := c.NovelRecommended(context.Background(), pixiv.NovelRecommendedRequest{})
			return err
		}, pixiv.OperationNovelRecommended},
		{"user envelope", "/v1/user/recommended", `{}`, func(c *pixiv.Client) error {
			_, err := c.UserRecommended(context.Background(), pixiv.UserRecommendedRequest{})
			return err
		}, pixiv.OperationUserRecommended},
		{"user id", "/v1/user/recommended", `{"user_previews":[{"user":{"id":0}}],"next_url":null}`, func(c *pixiv.Client) error {
			_, err := c.UserRecommended(context.Background(), pixiv.UserRecommendedRequest{})
			return err
		}, pixiv.OperationUserRecommended},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != test.path {
					t.Fatalf("path=%q", r.URL.Path)
				}
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "token"})
			if err != nil {
				t.Fatal(err)
			}
			err = test.call(client)
			var typed *pixiv.Error
			if !errors.Is(err, pixiv.ErrMalformedUpstreamResponse) || !errors.As(err, &typed) || typed.Operation != test.op || typed.Backend != pixiv.BackendAppAPI {
				t.Fatalf("err=%#v typed=%#v", err, typed)
			}
		})
	}
}

func TestRecommendationsRequireAuthenticationBeforeNetwork(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, WebFallbackEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() error{
		func() error {
			_, err := client.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
			return err
		},
		func() error {
			_, err := client.MangaRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
			return err
		},
		func() error {
			_, err := client.NovelRecommended(context.Background(), pixiv.NovelRecommendedRequest{})
			return err
		},
		func() error {
			_, err := client.UserRecommended(context.Background(), pixiv.UserRecommendedRequest{})
			return err
		},
	} {
		if err := call(); !errors.Is(err, pixiv.ErrUnauthorized) {
			t.Fatalf("unauthenticated recommendation error=%v", err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("unauthenticated recommendations used network: %d", requests.Load())
	}
}
