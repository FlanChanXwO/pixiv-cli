package pixiv_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
)

func TestRelatedAndTrendingExposeTypedFailuresWithoutFallbackOrSecrets(t *testing.T) {
	t.Parallel()
	operations := []struct {
		name string
		path string
		op   pixiv.Operation
		call func(*pixiv.Client) (any, error)
		id   int64
	}{
		{"related", "/v2/illust/related", pixiv.OperationIllustRelated, func(c *pixiv.Client) (any, error) {
			return c.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{IllustID: 731})
		}, 731},
		{"trending", "/v1/trending-tags/illust", pixiv.OperationTrendingTagsIllust, func(c *pixiv.Client) (any, error) { return c.TrendingTagsIllust(context.Background()) }, 0},
	}
	responses := []struct {
		name   string
		status int
		body   string
		code   pixiv.ErrorCode
		retry  bool
	}{
		{"http", http.StatusBadGateway, `body-secret-token`, pixiv.CodeUpstreamError, true},
		{"malformed", http.StatusOK, `{}`, pixiv.CodeMalformedUpstreamResponse, false},
	}
	for _, operation := range operations {
		operation := operation
		for _, response := range responses {
			response := response
			t.Run(operation.name+"/"+response.name, func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.URL.Path != operation.path {
						t.Fatalf("fallback path=%s", r.URL.Path)
					}
					w.WriteHeader(response.status)
					_, _ = w.Write([]byte(response.body))
				}))
				defer server.Close()
				client, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL, AccessToken: "access-secret"})
				result, err := operation.call(client)
				if !nilAtomResult(result) {
					t.Fatalf("partial=%+v", result)
				}
				upstreamStatus := response.status
				if response.status == http.StatusOK {
					upstreamStatus = 0
				}
				assertAtomError(t, err, response.code, operation.op, pixiv.BackendAppAPI, operation.id, upstreamStatus, response.retry, "body-secret-token", "access-secret")
				if calls != 1 {
					t.Fatalf("calls=%d", calls)
				}
			})
		}
	}
}

func TestRelatedAndTrendingExposeTransportAndContextFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		op        pixiv.Operation
		call      func(*pixiv.Client) (any, error)
		transport error
		retry     bool
		id        int64
	}{
		{"related transport", pixiv.OperationIllustRelated, func(c *pixiv.Client) (any, error) {
			return c.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{IllustID: 731})
		}, errors.New("https://proxy:proxy-password@example.invalid/?token=query-secret"), true, 731},
		{"related canceled", pixiv.OperationIllustRelated, func(c *pixiv.Client) (any, error) {
			return c.IllustRelated(context.Background(), pixiv.IllustRelatedRequest{IllustID: 731})
		}, context.Canceled, false, 731},
		{"trending transport", pixiv.OperationTrendingTagsIllust, func(c *pixiv.Client) (any, error) { return c.TrendingTagsIllust(context.Background()) }, errors.New("proxy-password query-secret"), true, 0},
		{"trending deadline", pixiv.OperationTrendingTagsIllust, func(c *pixiv.Client) (any, error) { return c.TrendingTagsIllust(context.Background()) }, context.DeadlineExceeded, false, 0},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			httpClient := &http.Client{Transport: atomErrorTransport{err: tt.transport}}
			client, _ := pixiv.NewClient(pixiv.Options{HTTPClient: httpClient, AppAPIBaseURL: "https://app.invalid", AccessToken: "token"})
			result, err := tt.call(client)
			if !nilAtomResult(result) {
				t.Fatalf("partial=%+v", result)
			}
			assertAtomError(t, err, pixiv.CodeUpstreamUnavailable, tt.op, pixiv.BackendAppAPI, tt.id, 0, tt.retry, "proxy-password", "query-secret")
			if errors.Is(tt.transport, context.Canceled) && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost canceled: %v", err)
			}
			if errors.Is(tt.transport, context.DeadlineExceeded) && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("lost deadline: %v", err)
			}
		})
	}
}

func TestUgoiraWebFailuresAreTypedSecretSafeAndReturnNoPartial(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		body       string
		transport  error
		code       pixiv.ErrorCode
		retry      bool
		contextErr error
	}{
		{"envelope", `{"error":true,"message":"message-secret"}`, nil, pixiv.CodeUpstreamError, true, nil},
		{"transport", "", errors.New("https://proxy:proxy-secret@example.invalid/?token=query-secret"), pixiv.CodeUpstreamUnavailable, true, nil},
		{"canceled", "", context.Canceled, pixiv.CodeUpstreamUnavailable, false, context.Canceled},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var httpClient *http.Client
			var base string
			if tt.transport != nil {
				httpClient, base = &http.Client{Transport: atomErrorTransport{err: tt.transport}}, "https://web.invalid"
			} else {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
						t.Fatal("secret header sent to Web")
					}
					_, _ = w.Write([]byte(tt.body))
				}))
				defer server.Close()
				httpClient, base = server.Client(), server.URL
			}
			client, _ := pixiv.NewClient(pixiv.Options{HTTPClient: httpClient, WebAPIBaseURL: base, WebFallbackEnabled: true})
			result, err := client.UgoiraMetadata(context.Background(), 731)
			if !nilAtomResult(result) {
				t.Fatalf("partial=%+v", result)
			}
			assertAtomError(t, err, tt.code, pixiv.OperationUgoiraMetadata, pixiv.BackendWebAPI, 731, 0, tt.retry, "message-secret", "proxy-secret", "query-secret")
			if tt.contextErr != nil && !errors.Is(err, tt.contextErr) {
				t.Fatalf("lost context: %v", err)
			}
		})
	}
}

func TestIllustPagesDistinguishesMissingNullAndExplicitEmptyBody(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, body string
		valid      bool
	}{{"missing", `{"error":false}`, false}, {"null", `{"error":false,"body":null}`, false}, {"empty", `{"error":false,"body":[]}`, true}} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tt.body)) }))
			defer server.Close()
			client, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), WebAPIBaseURL: server.URL, AccessToken: "token"})
			pages, err := client.IllustPages(context.Background(), 731)
			if tt.valid {
				if err != nil || pages == nil || len(pages) != 0 {
					t.Fatalf("pages=%+v err=%v", pages, err)
				}
				return
			}
			if pages != nil {
				t.Fatalf("partial=%+v", pages)
			}
			assertAtomError(t, err, pixiv.CodeMalformedUpstreamResponse, pixiv.OperationIllustPages, pixiv.BackendWebAPI, 731, 0, false)
		})
	}
}

type atomErrorTransport struct{ err error }

func (t atomErrorTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, t.err }

func assertAtomError(t *testing.T, err error, code pixiv.ErrorCode, operation pixiv.Operation, backend pixiv.Backend, illustID int64, status int, retry bool, secrets ...string) {
	t.Helper()
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != code || typed.Operation != operation || typed.Backend != backend || typed.IllustID != illustID || typed.UpstreamStatus != status || typed.Retryable != retry {
		t.Fatalf("err=%#v typed=%+v", err, typed)
	}
	rendered := fmt.Sprint(err) + " " + fmt.Sprint(errors.Unwrap(err))
	for _, secret := range secrets {
		if strings.Contains(rendered, secret) {
			t.Fatalf("secret %q leaked in %q", secret, rendered)
		}
	}
}

func nilAtomResult(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	return (v.Kind() == reflect.Pointer || v.Kind() == reflect.Slice) && v.IsNil()
}
