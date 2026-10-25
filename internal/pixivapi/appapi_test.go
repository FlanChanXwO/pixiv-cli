package pixivapi_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
)

type testDetailResponse struct {
	Illust struct {
		ID int64 `json:"id"`
	} `json:"illust"`
}

type testMutationResponse struct {
	CommentID int64 `json:"comment_id"`
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func getTestDetail(ctx context.Context, client *pixivapi.Client) (testDetailResponse, error) {
	var result testDetailResponse
	err := client.GetJSON(ctx, "/v1/illust/detail", url.Values{"illust_id": {"42"}}, &result)
	return result, err
}

type fakeSession struct {
	token        string
	refreshCalls int
	refreshErr   error
}

func (s *fakeSession) AccessToken() string { return s.token }
func (s *fakeSession) Refresh(context.Context) error {
	s.refreshCalls++
	if s.refreshErr != nil {
		return s.refreshErr
	}
	s.token = "new-access"
	return nil
}

// TestAppClientRequestContract 覆盖 App client 的请求形状、鉴权与响应解码。
func TestAppClientRequestContract(t *testing.T) {
	t.Run("get with bearer and user id", func(t *testing.T) {
		var received *http.Request
		client := pixivapi.New(pixivapi.Config{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				received = req.Clone(req.Context())
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"illust":{"id":42}}`))}, nil
			})},
			AccessToken: "direct-access", UserID: 42,
		})
		result, err := getTestDetail(context.Background(), client)
		if err != nil || result.Illust.ID != 42 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		if received.Header.Get("Authorization") != "Bearer direct-access" || received.Header.Get("X-User-Id") != "42" || received.Header.Get("User-Agent") != pixivapi.AppUserAgent {
			t.Fatalf("headers = %v", received.Header)
		}
	})

	t.Run("post form", func(t *testing.T) {
		var received *http.Request
		client := pixivapi.New(pixivapi.Config{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				received = req.Clone(req.Context())
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"comment_id":73}`))}, nil
			})},
			AccessToken: "direct-access",
		})
		var result testMutationResponse
		err := client.PostFormJSON(context.Background(), "/v1/illust/comment/add", url.Values{"illust_id": {"42"}, "comment": {"hello"}, "extra[]": {"a", "b"}}, &result)
		if err != nil || result.CommentID != 73 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		if err := received.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if received.Method != http.MethodPost || received.URL.Path != "/v1/illust/comment/add" ||
			received.PostForm.Get("illust_id") != "42" || received.PostForm.Get("comment") != "hello" || strings.Join(received.PostForm["extra[]"], ",") != "a,b" ||
			received.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || received.Header.Get("Authorization") != "Bearer direct-access" {
			t.Fatalf("request = %s %s form=%v headers=%v", received.Method, received.URL, received.PostForm, received.Header)
		}
	})

	t.Run("raw body", func(t *testing.T) {
		client := pixivapi.New(pixivapi.Config{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/v1/novel/content" || req.URL.Query().Get("novel_id") != "9" {
					t.Errorf("request = %s", req.URL)
				}
				if req.Header.Get("X-User-Id") != "42" {
					t.Errorf("X-User-Id = %q", req.Header.Get("X-User-Id"))
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("<p>raw</p>"))}, nil
			})},
			AccessToken: "access", UserID: 42,
		})
		body, err := client.GetRaw(context.Background(), "/v1/novel/content", url.Values{"novel_id": {"9"}})
		if err != nil || string(body) != "<p>raw</p>" {
			t.Fatalf("body = %q err=%v", body, err)
		}
	})
}

func TestAppClientMalformedSuccessResponse(t *testing.T) {
	for _, body := range []string{"", "   \n\t", "not-json"} {
		t.Run(strings.ReplaceAll(body, "\n", "\\n"), func(t *testing.T) {
			requests := 0
			client := pixivapi.New(pixivapi.Config{
				HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					requests++
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
				})},
				AccessToken: "access",
			})
			var result testMutationResponse
			err := client.PostFormJSON(context.Background(), "/v1/illust/comment/add", url.Values{"illust_id": {"42"}}, &result)
			if !errors.Is(err, pixivapi.ErrMalformedResponse) || requests != 1 {
				t.Fatalf("error = %v requests = %d", err, requests)
			}
		})
	}
}

func TestAppClientRefreshAndReplayContract(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run("refresh/"+http.StatusText(status), func(t *testing.T) {
			session := &fakeSession{token: "old-access"}
			requests := 0
			client := pixivapi.New(pixivapi.Config{
				HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					requests++
					if requests == 1 && req.Header.Get("Authorization") != "Bearer old-access" {
						t.Fatalf("first Authorization = %q", req.Header.Get("Authorization"))
					}
					if requests == 2 && req.Header.Get("Authorization") != "Bearer new-access" {
						t.Fatalf("retry Authorization = %q", req.Header.Get("Authorization"))
					}
					if requests == 1 {
						return &http.Response{StatusCode: status, Header: http.Header{}, Body: http.NoBody}, nil
					}
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"illust":{"id":42}}`))}, nil
				})},
				Session: session,
			})
			result, err := getTestDetail(context.Background(), client)
			if err != nil || result.Illust.ID != 42 || session.refreshCalls != 1 || requests != 2 {
				t.Fatalf("result=%+v err=%v refresh=%d requests=%d", result, err, session.refreshCalls, requests)
			}
		})
	}

	t.Run("refresh cancellation", func(t *testing.T) {
		session := &fakeSession{token: "old-access", refreshErr: context.Canceled}
		client := pixivapi.New(pixivapi.Config{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: http.NoBody}, nil
			})},
			Session: session,
		})
		if _, err := getTestDetail(context.Background(), client); !errors.Is(err, context.Canceled) || session.refreshCalls != 1 {
			t.Fatalf("error = %v refresh=%d", err, session.refreshCalls)
		}
	})

	t.Run("non-auth status with auth words", func(t *testing.T) {
		session := &fakeSession{token: "old-access"}
		requests := 0
		client := pixivapi.New(pixivapi.Config{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return &http.Response{StatusCode: http.StatusBadGateway, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("oauth unauthorized invalid_grant"))}, nil
			})},
			Session: session,
		})
		if _, err := getTestDetail(context.Background(), client); err == nil || requests != 1 || session.refreshCalls != 0 {
			t.Fatalf("err=%v requests=%d refresh=%d", err, requests, session.refreshCalls)
		}
	})

	t.Run("mutation refresh", func(t *testing.T) {
		session := &fakeSession{token: "old-access"}
		requests := 0
		client := pixivapi.New(pixivapi.Config{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if err := req.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				if req.Method != http.MethodPost || req.URL.Path != "/v2/illust/bookmark/add" ||
					req.PostForm.Get("illust_id") != "42" || req.PostForm.Get("restrict") != "public" || len(req.PostForm["tags[]"]) != 2 {
					t.Fatalf("form = %v", req.PostForm)
				}
				if req.Header.Get("Authorization") != "Bearer new-access" {
					return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: http.NoBody}, nil
				}
				return &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{}, Body: http.NoBody}, nil
			})},
			Session: session,
		})
		if err := client.PostForm(context.Background(), "/v2/illust/bookmark/add", url.Values{"illust_id": {"42"}, "restrict": {"public"}, "tags[]": {"a", "b"}}); err != nil || session.refreshCalls != 1 {
			t.Fatalf("err=%v refresh=%d", err, session.refreshCalls)
		}
	})
}

func TestAppClientUncertainFailuresAreNotReplayed(t *testing.T) {
	for name, test := range map[string]struct {
		status     int
		retryAfter string
	}{
		"rate-limit":   {status: http.StatusTooManyRequests, retryAfter: "0"},
		"server-error": {status: http.StatusBadGateway},
	} {
		t.Run(name, func(t *testing.T) {
			session := &fakeSession{token: "access"}
			requests := 0
			client := pixivapi.New(pixivapi.Config{
				HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					requests++
					if req.Method == http.MethodGet {
						header := http.Header{}
						if test.retryAfter != "" {
							header.Set("Retry-After", test.retryAfter)
						}
						return &http.Response{StatusCode: test.status, Header: header, Body: http.NoBody}, nil
					}
					header := http.Header{}
					if test.retryAfter != "" {
						header.Set("Retry-After", test.retryAfter)
					}
					return &http.Response{StatusCode: test.status, Header: header, Body: io.NopCloser(strings.NewReader(`{"error":"request outcome is unknown"}`))}, nil
				})},
				Session: session,
			})
			var result testMutationResponse
			if err := client.PostFormJSON(context.Background(), "/v1/illust/comment/add", url.Values{"illust_id": {"42"}}, &result); err == nil || session.refreshCalls != 0 {
				t.Fatalf("mutation err=%v refresh=%d", err, session.refreshCalls)
			}
			if requests != 1 {
				t.Fatalf("mutation requests = %d, want 1", requests)
			}
		})
	}

	t.Run("server error mutation", func(t *testing.T) {
		session := &fakeSession{token: "old-access"}
		requests := 0
		client := pixivapi.New(pixivapi.Config{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return &http.Response{StatusCode: http.StatusBadGateway, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("unauthorized"))}, nil
			})},
			Session: session,
		})
		if err := client.PostForm(context.Background(), "/v1/user/follow/delete", url.Values{"user_id": {"42"}}); err == nil || requests != 1 || session.refreshCalls != 0 {
			t.Fatalf("err=%v requests=%d refresh=%d", err, requests, session.refreshCalls)
		}
	})

	t.Run("transport", func(t *testing.T) {
		calls := 0
		client := pixivapi.New(pixivapi.Config{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("connection reset")
			})},
			AccessToken: "access",
		})
		var result testMutationResponse
		if err := client.PostFormJSON(context.Background(), "/v1/illust/comment/add", url.Values{"illust_id": {"42"}}, &result); err == nil || calls != 1 {
			t.Fatalf("err=%v calls=%d", err, calls)
		}
	})
}

func TestAppClientRateLimitPolicy(t *testing.T) {
	detail := func(requests *int, status func(int) (int, string)) *pixivapi.Client {
		return pixivapi.New(pixivapi.Config{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				*requests++
				code, retryAfter := status(*requests)
				header := http.Header{}
				if retryAfter != "" {
					header.Set("Retry-After", retryAfter)
				}
				if code == http.StatusOK {
					return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(`{"illust":{"id":42}}`))}, nil
				}
				return &http.Response{StatusCode: code, Header: header, Body: http.NoBody}, nil
			})},
			AccessToken: "access",
		})
	}

	t.Run("retry once with valid retry-after", func(t *testing.T) {
		requests := 0
		client := detail(&requests, func(call int) (int, string) {
			if call == 1 {
				return http.StatusTooManyRequests, "0"
			}
			return http.StatusOK, ""
		})
		result, err := getTestDetail(context.Background(), client)
		if err != nil || result.Illust.ID != 42 || requests != 2 {
			t.Fatalf("result=%+v err=%v requests=%d", result, err, requests)
		}
	})

	for _, header := range []string{"", "not-a-delay", "-1"} {
		t.Run("invalid retry-after/"+header, func(t *testing.T) {
			requests := 0
			client := detail(&requests, func(int) (int, string) { return http.StatusTooManyRequests, header })
			if _, err := getTestDetail(context.Background(), client); err == nil || requests != 1 {
				t.Fatalf("err=%v requests=%d", err, requests)
			}
		})
	}

	t.Run("stop after second rate limit", func(t *testing.T) {
		requests := 0
		client := detail(&requests, func(int) (int, string) { return http.StatusTooManyRequests, "0" })
		if _, err := getTestDetail(context.Background(), client); err == nil || requests != 2 {
			t.Fatalf("err=%v requests=%d", err, requests)
		}
	})

	t.Run("wait honors cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		requests := 0
		client := pixivapi.New(pixivapi.Config{
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests++
				cancel()
				return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"60"}}, Body: http.NoBody}, nil
			})},
			AccessToken: "access",
		})
		if _, err := getTestDetail(ctx, client); !errors.Is(err, context.Canceled) || requests != 1 {
			t.Fatalf("err=%v requests=%d", err, requests)
		}
	})
}

func TestAppClientPreservesCallerClientAndCancellation(t *testing.T) {
	t.Run("explicit client", func(t *testing.T) {
		calls := 0
		client := pixivapi.New(pixivapi.Config{
			BaseURL: "https://example.invalid",
			HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"illust":{"id":42}}`)), Request: req}, nil
			})},
			AccessToken: "access",
		})
		if result, err := getTestDetail(context.Background(), client); err != nil || result.Illust.ID != 42 || calls != 1 {
			t.Fatalf("result=%+v err=%v calls=%d", result, err, calls)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
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
			result <- pixivapi.New(pixivapi.Config{BaseURL: api.URL, AccessToken: "access"}).GetJSON(ctx, "/v1/illust/detail", nil, &response)
		}()
		<-started
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("GetJSON error = %v, want context.Canceled", err)
		}
	})
}

func TestAppClientLogsOmitSecretsAndQueries(t *testing.T) {
	const (
		accessToken = "access-token-canary"
		querySecret = "query-canary"
		bodySecret  = "body-canary"
	)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("word") != querySecret || r.Header.Get("Authorization") != "Bearer "+accessToken {
			t.Fatalf("request = %s auth=%q", r.URL, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"secret":"` + bodySecret + `"}`))
	}))
	defer api.Close()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := logging.WithOperation(context.Background(), "SearchArtworks")
	var result testDetailResponse
	err := pixivapi.New(pixivapi.Config{BaseURL: api.URL, HTTPClient: api.Client(), AccessToken: accessToken, Logger: logger}).
		GetJSON(ctx, "/v1/illust/detail", url.Values{"word": {querySecret}}, &result)
	if err != nil {
		t.Fatal(err)
	}
	logged := output.String()
	for _, want := range []string{`"method":"GET"`, `"path":"/v1/illust/detail"`, `"status":200`, `"sdk_operation":"SearchArtworks"`} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log missing %s: %s", want, logged)
		}
	}
	for _, secret := range []string{accessToken, querySecret, bodySecret} {
		if strings.Contains(logged, secret) {
			t.Fatalf("log leaked %q: %s", secret, logged)
		}
	}
}
