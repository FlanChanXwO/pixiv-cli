package appapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
)

func TestNewLeavesRequestLifetimeToContext(t *testing.T) {
	client := New()
	httpClient := client.restyClient.GetClient()
	if httpClient == http.DefaultClient {
		t.Fatal("HTTP client unexpectedly aliases http.DefaultClient")
	}
	if httpClient.Timeout != 0 {
		t.Fatalf("timeout = %v, want zero", httpClient.Timeout)
	}
}

func TestNewPreservesExplicitHTTPClient(t *testing.T) {
	want := &http.Client{Timeout: 17 * time.Second}
	got := New(WithHTTPClient(want)).restyClient.GetClient()
	if got != want || got.Timeout != want.Timeout {
		t.Fatalf("HTTP client = %p timeout %v, want %p timeout %v", got, got.Timeout, want, want.Timeout)
	}
}

func TestIllustDetailPreservesCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := New(WithBaseURL("https://example.invalid"), WithAccessToken("access")).IllustDetail(ctx, 42)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestSearchIllustMapsNormalizedFiltersToAppQuery(t *testing.T) {
	tests := []struct {
		name        string
		filters     model.SearchIllustFilters
		wantQuery   map[string]string
		absentQuery []string
	}{
		{
			name: "defaults omitted", filters: model.SearchIllustFilters{Rating: "all", ContentType: "all", AIMode: "all", AspectRatio: "all", Resolution: "all"},
			wantQuery:   map[string]string{"search_ai_type": "0"},
			absentQuery: []string{"ratio_pattern", "content_type", "width_min", "width_max", "height_min", "height_max", "tool"},
		},
		{
			name: "only AI square low illustration and ugoira", filters: model.SearchIllustFilters{
				ContentType: "illust-and-ugoira", AIMode: "only", AspectRatio: "square", Resolution: "low", Tool: "tool",
			},
			wantQuery:   map[string]string{"search_ai_type": "0", "content_type": "illust_and_ugoira", "ratio_pattern": "square", "width_max": "999", "height_max": "999", "tool": "tool"},
			absentQuery: []string{"width_min", "height_min"},
		},
		{
			name: "exclude AI portrait high ugoira", filters: model.SearchIllustFilters{
				ContentType: "ugoira", AIMode: "exclude", AspectRatio: "portrait", Resolution: "high",
			},
			wantQuery:   map[string]string{"content_type": "ugoira", "search_ai_type": "1", "ratio_pattern": "portrait", "width_min": "3000", "height_min": "3000"},
			absentQuery: []string{"width_max", "height_max"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, value := range test.wantQuery {
					if got := r.URL.Query().Get(key); got != value {
						t.Fatalf("%s = %q, want %q; query=%v", key, got, value, r.URL.Query())
					}
				}
				for _, key := range test.absentQuery {
					if r.URL.Query().Has(key) {
						t.Fatalf("query unexpectedly has %q: %v", key, r.URL.Query())
					}
				}
				_, _ = w.Write([]byte(`{"illusts":[]}`))
			}))
			defer api.Close()
			_, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).SearchIllust(
				context.Background(), "miku", "partial_match_for_tags", "date_desc", "", "", "", 0, test.filters,
			)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSearchIllustMapsDateAndBookmarkBoundsToAppQuery(t *testing.T) {
	minimum, maximum := 1000, 10000
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		want := map[string]string{
			"search_target":    "keyword",
			"start_date":       "2026-01-01",
			"end_date":         "2026-01-31",
			"bookmark_num_min": "1000",
			"bookmark_num_max": "10000",
		}
		for key, value := range want {
			if got := query.Get(key); got != value {
				t.Fatalf("%s = %q, want %q; query=%v", key, got, value, query)
			}
		}
		_, _ = w.Write([]byte(`{"illusts":[]}`))
	}))
	defer api.Close()
	_, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).SearchIllust(
		context.Background(), "miku", "keyword", "date_desc", "", "2026-01-01", "2026-01-31", 0,
		model.SearchIllustFilters{BookmarkMin: &minimum, BookmarkMax: &maximum},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRefreshAndRetryOnceOnTypedAuthStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			session := &fakeSession{token: "old-access"}
			requests := 0
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v1/illust/detail" {
					t.Fatalf("unexpected api path %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer new-access" {
					http.Error(w, `{"error":{"message":"auth required"}}`, status)
					return
				}
				_ = json.NewEncoder(w).Encode(model.IllustDetail{Illust: model.Illust{ID: 42, Title: "ok"}})
			}))
			defer api.Close()

			client := New(WithBaseURL(api.URL), WithSession(session))
			result, err := client.IllustDetail(context.Background(), 42)
			if err != nil {
				t.Fatalf("IllustDetail returned error: %v", err)
			}
			if result.Illust.ID != 42 || session.refreshCalls != 1 || requests != 2 {
				t.Fatalf("result=%+v refresh calls=%d requests=%d", result, session.refreshCalls, requests)
			}
		})
	}
}

func TestGETNonAuthStatusDoesNotRefreshOrReplayForAuthWordsInBody(t *testing.T) {
	session := &fakeSession{token: "old-access"}
	requests := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/illust/detail" {
			t.Fatalf("unexpected api path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("oauth unauthorized invalid_grant"))
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithSession(session))
	if _, err := client.IllustDetail(context.Background(), 42); err == nil {
		t.Fatal("IllustDetail unexpectedly succeeded")
	}
	if requests != 1 || session.refreshCalls != 0 {
		t.Fatalf("requests=%d refresh calls=%d", requests, session.refreshCalls)
	}
}

func TestGETRetriesOnceAfterValidRateLimitResponse(t *testing.T) {
	requests := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/v1/illust/detail" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(model.IllustDetail{Illust: model.Illust{ID: 42}})
	}))
	defer api.Close()

	result, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).IllustDetail(context.Background(), 42)
	if err != nil || result.Illust.ID != 42 || requests != 2 {
		t.Fatalf("result=%#v err=%v requests=%d", result, err, requests)
	}
}

func TestGETDoesNotRetryRateLimitWithoutValidRetryAfter(t *testing.T) {
	for _, header := range []string{"", "not-a-delay", "-1"} {
		t.Run(header, func(t *testing.T) {
			requests := 0
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				if header != "" {
					w.Header().Set("Retry-After", header)
				}
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			defer api.Close()

			_, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).IllustDetail(context.Background(), 42)
			if err == nil || requests != 1 {
				t.Fatalf("error=%v requests=%d", err, requests)
			}
		})
	}
}

func TestGETStopsAfterSecondRateLimitResponse(t *testing.T) {
	requests := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer api.Close()

	_, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).IllustDetail(context.Background(), 42)
	if err == nil || requests != 2 {
		t.Fatalf("error=%v requests=%d", err, requests)
	}
}

func TestGETRateLimitWaitHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		cancel()
	}))
	defer api.Close()

	_, err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).IllustDetail(ctx, 42)
	if !errors.Is(err, context.Canceled) || requests != 1 {
		t.Fatalf("error=%v requests=%d", err, requests)
	}
}

func TestMutationRateLimitIsNotReplayed(t *testing.T) {
	requests := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer api.Close()

	err := New(WithBaseURL(api.URL), WithHTTPClient(api.Client()), WithAccessToken("access")).AddBookmark(context.Background(), 42, "public", nil)
	if err == nil || requests != 1 {
		t.Fatalf("error=%v requests=%d", err, requests)
	}
}

func TestParseRetryAfterSupportsSecondsAndHTTPDate(t *testing.T) {
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		value string
		want  time.Duration
		ok    bool
	}{
		{value: "5", want: 5 * time.Second, ok: true},
		{value: now.Add(3 * time.Second).Format(http.TimeFormat), want: 3 * time.Second, ok: true},
		{value: now.Add(-time.Second).Format(http.TimeFormat), want: 0, ok: true},
		{value: "invalid", ok: false},
		{value: "9223372036854775807", ok: false}, // Go duration 无法表达的秒数不是可用等待值。
	} {
		got, ok := parseRetryAfter(test.value, now)
		if ok != test.ok || got != test.want {
			t.Fatalf("parseRetryAfter(%q) = (%v, %t), want (%v, %t)", test.value, got, ok, test.want, test.ok)
		}
	}
}

type fakeSession struct {
	token        string
	refreshCalls int
}

func (s *fakeSession) AccessToken() string { return s.token }
func (s *fakeSession) Refresh(context.Context) error {
	s.refreshCalls++
	s.token = "new-access"
	return nil
}

func TestUserDetailFetchesUserName(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/detail" {
			t.Fatalf("unexpected api path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("user_id"); got != "123" {
			t.Fatalf("user_id = %q, want 123", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user":              map[string]any{"id": 123, "name": "alice"},
			"profile":           map[string]any{},
			"profile_publicity": map[string]any{"gender": false, "region": false, "birth_day": false, "birth_year": false, "job": false, "pawoo": false},
			"workspace":         map[string]any{},
		})
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithAccessToken("access"))
	detail, err := client.UserDetail(context.Background(), 123)
	if err != nil {
		t.Fatalf("UserDetail returned error: %v", err)
	}
	if detail.User.ID != 123 || detail.User.Name != "alice" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestUserDetailNormalizesProfilePublicityWireValues(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/detail" {
			t.Fatalf("unexpected api path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"user":{"id":123},
			"profile":{},
			"profile_publicity":{"gender":"pub\u006cic","region":"private","birth_day":true,"birth_year":false,"job":"public","pawoo":true},
			"workspace":{}
		}`))
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithAccessToken("access"))
	detail, err := client.UserDetail(context.Background(), 123)
	if err != nil {
		t.Fatalf("UserDetail returned error: %v", err)
	}
	got := detail.ProfilePublicity
	if !got.Gender || got.Region || !got.BirthDay || got.BirthYear || !got.Job || !got.Pawoo {
		t.Fatalf("profile publicity = %#v", got)
	}
}

func TestUserDetailNormalizesMissingProfilePublicityFieldsToFalse(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"user":{"id":123},
			"profile":{},
			"profile_publicity":{"gender":"public"},
			"workspace":{}
		}`))
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithAccessToken("access"))
	detail, err := client.UserDetail(context.Background(), 123)
	if err != nil {
		t.Fatalf("UserDetail returned error: %v", err)
	}
	got := detail.ProfilePublicity
	if !got.Gender || got.Region || got.BirthDay || got.BirthYear || got.Job || got.Pawoo {
		t.Fatalf("profile publicity = %#v", got)
	}
}

func TestUserDetailRejectsMalformedProfilePublicityWireValues(t *testing.T) {
	tests := []struct {
		name      string
		publicity string
	}{
		{name: "unknown string", publicity: `{"gender":"friends","region":true,"birth_day":true,"birth_year":true,"job":true,"pawoo":true}`},
		{name: "null", publicity: `{"gender":null,"region":true,"birth_day":true,"birth_year":true,"job":true,"pawoo":true}`},
		{name: "number", publicity: `{"gender":1,"region":true,"birth_day":true,"birth_year":true,"job":true,"pawoo":true}`},
		{name: "array", publicity: `{"gender":[],"region":true,"birth_day":true,"birth_year":true,"job":true,"pawoo":true}`},
		{name: "object", publicity: `{"gender":{},"region":true,"birth_day":true,"birth_year":true,"job":true,"pawoo":true}`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"user":{"id":123},"profile":{},"profile_publicity":` + test.publicity + `,"workspace":{}}`))
			}))
			defer api.Close()

			client := New(WithBaseURL(api.URL), WithAccessToken("access"))
			detail, err := client.UserDetail(context.Background(), 123)
			if detail != nil || !errors.Is(err, ErrMalformedResponse) {
				t.Fatalf("detail=%#v err=%v", detail, err)
			}
		})
	}
}

func TestWithAccessTokenSendsBearerAuthorization(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer direct-access" {
			t.Fatalf("Authorization = %q, want bearer access token", got)
		}
		_ = json.NewEncoder(w).Encode(model.IllustDetail{Illust: model.Illust{ID: 42}})
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithAccessToken("direct-access"))
	if _, err := client.IllustDetail(context.Background(), 42); err != nil {
		t.Fatalf("IllustDetail returned error: %v", err)
	}
}

func TestBookmarkMutationRefreshesAuthAndAcceptsEmptySuccess(t *testing.T) {
	session := &fakeSession{token: "old-access"}
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v2/illust/bookmark/add" {
			t.Fatalf("method=%s path=%s", request.Method, request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if request.Form.Get("illust_id") != "42" || request.Form.Get("restrict") != "public" || len(request.Form["tags[]"]) != 2 {
			t.Fatalf("form=%v", request.Form)
		}
		if request.Header.Get("Authorization") != "Bearer new-access" {
			http.Error(writer, `{"error":{"message":"unauthorized"}}`, http.StatusUnauthorized)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithSession(session))
	if err := client.AddBookmark(context.Background(), 42, "public", []string{"a", "b"}); err != nil {
		t.Fatalf("AddBookmark returned error: %v", err)
	}
	if session.refreshCalls != 1 {
		t.Fatalf("refresh calls=%d", session.refreshCalls)
	}
}

func TestMutationServerErrorDoesNotRefreshOrReplay(t *testing.T) {
	session := &fakeSession{token: "old-access"}
	requests := 0
	api := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodPost || request.URL.Path != "/v1/user/follow/delete" {
			t.Fatalf("method=%s path=%s", request.Method, request.URL.Path)
		}
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte("unauthorized"))
	}))
	defer api.Close()

	client := New(WithBaseURL(api.URL), WithSession(session))
	if err := client.UnfollowUser(context.Background(), 42); err == nil {
		t.Fatal("UnfollowUser unexpectedly succeeded")
	}
	if requests != 1 || session.refreshCalls != 0 {
		t.Fatalf("requests=%d refresh_calls=%d", requests, session.refreshCalls)
	}
}
