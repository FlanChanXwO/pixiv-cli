package appapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	appapi "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/appapi"
)

type testDetailResponse struct {
	Illust struct {
		ID int64 `json:"id"`
	} `json:"illust"`
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func getTestDetail(ctx context.Context, client *appapi.Client) (testDetailResponse, error) {
	var result testDetailResponse
	err := client.GetJSON(ctx, "/v1/illust/detail", url.Values{"illust_id": {"42"}}, &result)
	return result, err
}

func TestGetRawPreservesBodyAndHeadersForEndpointFamily(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/novel/content" || r.URL.Query().Get("novel_id") != "9" {
			t.Fatalf("request = %s", r.URL.String())
		}
		if r.Header.Get("X-User-Id") != "42" {
			t.Fatalf("X-User-Id = %q", r.Header.Get("X-User-Id"))
		}
		_, _ = w.Write([]byte("<p>raw</p>"))
	}))
	defer api.Close()

	body, err := appapi.New(appapi.WithBaseURL(api.URL), appapi.WithHTTPClient(api.Client()), appapi.WithAccessToken("access"), appapi.WithUserID(42)).GetRaw(context.Background(), "/v1/novel/content", url.Values{"novel_id": {"9"}})
	if err != nil {
		t.Fatalf("GetRaw returned error: %v", err)
	}
	if string(body) != "<p>raw</p>" {
		t.Fatalf("body = %q", body)
	}
}

func TestNewPropagatesContextCancellation(t *testing.T) {
	started := make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer api.Close()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		var response testDetailResponse
		result <- appapi.New(appapi.WithBaseURL(api.URL), appapi.WithAccessToken("access")).GetJSON(ctx, "/v1/illust/detail", nil, &response)
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("GetJSON error = %v, want context.Canceled", err)
	}
}

func TestNewPreservesExplicitHTTPClient(t *testing.T) {
	calls := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"illust":{"id":42}}`)),
			Request:    request,
		}, nil
	})}
	result, err := getTestDetail(context.Background(), appapi.New(appapi.WithBaseURL("https://example.invalid"), appapi.WithHTTPClient(httpClient), appapi.WithAccessToken("access")))
	if err != nil || result.Illust.ID != 42 || calls != 1 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, calls)
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
				_ = json.NewEncoder(w).Encode(map[string]any{"illust": map[string]any{"id": 42}})
			}))
			defer api.Close()

			client := appapi.New(appapi.WithBaseURL(api.URL), appapi.WithSession(session))
			result, err := getTestDetail(context.Background(), client)
			if err != nil {
				t.Fatalf("GetJSON returned error: %v", err)
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

	client := appapi.New(appapi.WithBaseURL(api.URL), appapi.WithSession(session))
	if _, err := getTestDetail(context.Background(), client); err == nil {
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
		_ = json.NewEncoder(w).Encode(map[string]any{"illust": map[string]any{"id": 42}})
	}))
	defer api.Close()

	result, err := getTestDetail(context.Background(), appapi.New(appapi.WithBaseURL(api.URL), appapi.WithHTTPClient(api.Client()), appapi.WithAccessToken("access")))
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

			_, err := getTestDetail(context.Background(), appapi.New(appapi.WithBaseURL(api.URL), appapi.WithHTTPClient(api.Client()), appapi.WithAccessToken("access")))
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

	_, err := getTestDetail(context.Background(), appapi.New(appapi.WithBaseURL(api.URL), appapi.WithHTTPClient(api.Client()), appapi.WithAccessToken("access")))
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

	_, err := getTestDetail(ctx, appapi.New(appapi.WithBaseURL(api.URL), appapi.WithHTTPClient(api.Client()), appapi.WithAccessToken("access")))
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

	err := appapi.New(appapi.WithBaseURL(api.URL), appapi.WithHTTPClient(api.Client()), appapi.WithAccessToken("access")).PostForm(context.Background(), "/v2/illust/bookmark/add", url.Values{"illust_id": {"42"}, "restrict": {"public"}})
	if err == nil || requests != 1 {
		t.Fatalf("error=%v requests=%d", err, requests)
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

func TestWithAccessTokenSendsBearerAuthorization(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer direct-access" {
			t.Fatalf("Authorization = %q, want bearer access token", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"illust": map[string]any{"id": 42}})
	}))
	defer api.Close()

	client := appapi.New(appapi.WithBaseURL(api.URL), appapi.WithAccessToken("direct-access"))
	if _, err := getTestDetail(context.Background(), client); err != nil {
		t.Fatalf("GetJSON returned error: %v", err)
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

	client := appapi.New(appapi.WithBaseURL(api.URL), appapi.WithSession(session))
	if err := client.PostForm(context.Background(), "/v2/illust/bookmark/add", url.Values{"illust_id": {"42"}, "restrict": {"public"}, "tags[]": {"a", "b"}}); err != nil {
		t.Fatalf("PostForm returned error: %v", err)
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

	client := appapi.New(appapi.WithBaseURL(api.URL), appapi.WithSession(session))
	if err := client.PostForm(context.Background(), "/v1/user/follow/delete", url.Values{"user_id": {"42"}}); err == nil {
		t.Fatal("follow delete unexpectedly succeeded")
	}
	if requests != 1 || session.refreshCalls != 0 {
		t.Fatalf("requests=%d refresh_calls=%d", requests, session.refreshCalls)
	}
}
