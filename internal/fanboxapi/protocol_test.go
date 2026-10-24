package fanboxapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

// newTestSession 启动一个信任自签证书的 httptest TLS server，并把任意 fanbox/媒体
// 主机名的连接重定向到该 server，从而用标准 net/http 全栈测试 allowlist 行为。
func newTestSession(t *testing.T, handler http.Handler, cookie string) *fanboxapi.Session {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	base := server.Client().Transport.(*http.Transport)
	tlsConfig := base.TLSClientConfig.Clone()
	if tlsConfig.ServerName == "" {
		tlsConfig.ServerName = "example.com"
	}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
	session, err := fanboxapi.NewSessionWithOptions(cookie, fanboxapi.SessionOptions{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatalf("fanboxapi.NewSessionWithOptions() error = %v", err)
	}
	return session
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

func writeStatus(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

type postResponse struct {
	Body struct {
		Post struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"post"`
	} `json:"body"`
}

func fetchPost(session *fanboxapi.Session, ctx context.Context, postID string) (postResponse, error) {
	var response postResponse
	endpoint := fanboxapi.APIBaseURL + fanboxapi.PathPostInfo + "?" + url.Values{fanboxapi.ParamPostID: {postID}}.Encode()
	err := session.GetJSON(ctx, endpoint, &response)
	return response, err
}

func fetchPage(session *fanboxapi.Session, ctx context.Context, nextURL string) error {
	endpoint := nextURL
	if endpoint == "" {
		endpoint = fanboxapi.APIBaseURL + fanboxapi.PathPostListHome
	}
	var body struct {
		Body json.RawMessage `json:"body"`
	}
	return session.GetJSON(ctx, endpoint, &body)
}

func TestSessionIdentityAndStatusClassification(t *testing.T) {
	identity := `<meta name="metadata" content='{"context":{"user":{"userId":"800","name":"verified","creatorId":"artist","creatorStatus":"explicit","isCreator":true}}}'>`
	t.Run("identity", func(t *testing.T) {
		session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if request.Host != "www.fanbox.cc" || request.URL.Path != "/" || request.Header.Get("Cookie") != "FANBOXSESSID=identity-canary" || request.Header.Get("Origin") != "https://www.fanbox.cc" {
				t.Errorf("request host=%q url=%q cookie=%q origin=%q", request.Host, request.URL, request.Header.Get("Cookie"), request.Header.Get("Origin"))
			}
			_, _ = io.WriteString(w, identity)
		}), "FANBOXSESSID=identity-canary")
		got, err := session.CurrentUser(context.Background())
		want := fanboxapi.Identity{UserID: 800, DisplayName: "verified", CreatorID: "artist", CreatorStatus: "explicit", IsCreator: true}
		if err != nil || got != want {
			t.Fatalf("identity = %+v err=%v, want %+v", got, err, want)
		}
	})

	t.Run("missing metadata", func(t *testing.T) {
		session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "<html><head><title>fanbox</title></head></html>")
		}), "FANBOXSESSID=identity-canary")
		if _, err := session.CurrentUser(context.Background()); err == nil {
			t.Fatal("CurrentUser() unexpectedly succeeded without metadata")
		}
	})

	t.Run("status classification", func(t *testing.T) {
		session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/unauthorized":
				writeStatus(w, http.StatusUnauthorized, `{"error":"session expired"}`)
			case "/forbidden":
				writeStatus(w, http.StatusForbidden, `{"error":"access denied"}`)
			case "/challenge-body":
				writeStatus(w, http.StatusForbidden, `cf-chl: please enable javascript`)
			case "/challenge-html":
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Server", "cloudflare")
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `Just a moment...`)
			case "/challenge-header":
				w.Header().Set("Cf-Mitigated", "challenge")
				writeStatus(w, http.StatusForbidden, `{"error":"denied"}`)
			case "/notfound":
				writeStatus(w, http.StatusNotFound, `{"error":"missing"}`)
			default:
				t.Errorf("unexpected request %q", request.URL)
				writeStatus(w, http.StatusNotFound, `{}`)
			}
		}), "FANBOXSESSID=status-canary")
		for _, test := range []struct {
			path string
			want error
		}{
			{"/unauthorized", fanboxapi.ErrNotAuthenticated},
			{"/forbidden", fanboxapi.ErrForbidden},
			{"/challenge-body", fanboxapi.ErrChallenge},
			{"/challenge-html", fanboxapi.ErrChallenge},
			{"/challenge-header", fanboxapi.ErrChallenge},
			{"/notfound", nil},
		} {
			t.Run(test.path, func(t *testing.T) {
				err := fetchPage(session, context.Background(), "https://api.fanbox.cc"+test.path)
				if test.want == nil {
					if err == nil || errors.Is(err, fanboxapi.ErrForbidden) || errors.Is(err, fanboxapi.ErrChallenge) || errors.Is(err, fanboxapi.ErrNotAuthenticated) {
						t.Fatalf("generic status = %v", err)
					}
					return
				}
				if !errors.Is(err, test.want) {
					t.Fatalf("error = %v, want %v", err, test.want)
				}
			})
		}
	})
}

func TestSessionRejectsUnsafeURLsAndImplicitTransport(t *testing.T) {
	called := false
	session, err := fanboxapi.NewSessionWithOptions("FANBOXSESSID=url-canary", fanboxapi.SessionOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		"http://downloads.fanbox.cc/media.jpg",
		"https://user@downloads.fanbox.cc/media.jpg",
		"https://example.invalid/media.jpg",
	} {
		if _, err := session.OpenMedia(context.Background(), target); err == nil || strings.Contains(err.Error(), "url-canary") {
			t.Fatalf("OpenMedia(%q) = %v", target, err)
		}
	}
	if called {
		t.Fatal("transport was called for an unsafe URL")
	}

	client, err := fanboxapi.NewSessionWithOptions("FANBOXSESSID=tls-canary", fanboxapi.SessionOptions{})
	if err != nil {
		t.Fatalf("NewSessionWithOptions() error = %v", err)
	}
	defer client.CloseIdleConnections()
	if _, err := fanboxapi.NewSessionWithOptions("FANBOXSESSID=tls-canary", fanboxapi.SessionOptions{HTTPClient: &http.Client{}}); err == nil {
		t.Fatal("NewSessionWithOptions() accepted implicit standard transport")
	}
}

func TestSessionUsesCustomUserAgent(t *testing.T) {
	session, err := fanboxapi.NewSessionWithOptions("FANBOXSESSID=ua-canary", fanboxapi.SessionOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if got := request.Header.Get("User-Agent"); got != "custom-agent" {
				t.Errorf("User-Agent = %q", got)
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok"))}, nil
		})},
		UserAgent: "custom-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fetchPage(session, context.Background(), "https://api.fanbox.cc/post.info?postId=1"); err == nil {
		t.Fatal("expected decode failure after header assertion, got nil")
	}
}

// TestMediaOpenCredentialAndConditionalContract 覆盖媒体下载的 cookie 边界、
// redirect 丢弃会话与 304 条件 GET。
func TestMediaOpenCredentialAndConditionalContract(t *testing.T) {
	t.Run("redirect drops session", func(t *testing.T) {
		var seen []string
		session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			seen = append(seen, request.Host+":"+request.Header.Get("Cookie"))
			if request.Host == "downloads.fanbox.cc" {
				if request.Header.Get("Cookie") != "FANBOXSESSID=media-canary" {
					t.Errorf("downloads Cookie = %q", request.Header.Get("Cookie"))
				}
				w.Header().Set("Location", "https://i.pximg.net/media/asset.jpg")
				w.WriteHeader(http.StatusFound)
				return
			}
			if request.Header.Get("Cookie") != "" {
				t.Errorf("redirected media Cookie = %q", request.Header.Get("Cookie"))
			}
			_, _ = io.WriteString(w, "media")
		}), "FANBOXSESSID=media-canary")
		response, err := session.OpenMedia(context.Background(), "https://downloads.fanbox.cc/media/asset.jpg")
		if err != nil {
			t.Fatalf("OpenMedia() error = %v", err)
		}
		body, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil || string(body) != "media" {
			t.Fatalf("body = %q errors = %v/%v", body, readErr, closeErr)
		}
		if got, want := strings.Join(seen, ","), "downloads.fanbox.cc:FANBOXSESSID=media-canary,i.pximg.net:"; got != want {
			t.Fatalf("requests = %q, want %q", got, want)
		}
	})

	t.Run("api redirect drops session", func(t *testing.T) {
		var seen []string
		session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			seen = append(seen, request.Host+request.URL.Path+":"+request.Header.Get("Cookie"))
			switch request.Host + request.URL.Path {
			case "api.fanbox.cc/post.info":
				w.Header().Set("Location", "https://www.fanbox.cc/post-info")
				w.WriteHeader(http.StatusFound)
			case "www.fanbox.cc/post-info":
				writeJSON(w, `{"body":{"post":{"id":"post-1","title":"title"}}}`)
			default:
				t.Errorf("unexpected request %q host=%q", request.URL, request.Host)
				writeStatus(w, http.StatusNotFound, `{}`)
			}
		}), "FANBOXSESSID=redirect-canary")
		if _, err := fetchPost(session, context.Background(), "post-1"); err != nil {
			t.Fatalf("Post() error = %v", err)
		}
		if got, want := strings.Join(seen, ","), "api.fanbox.cc/post.info:FANBOXSESSID=redirect-canary,www.fanbox.cc/post-info:"; got != want {
			t.Fatalf("requests = %q, want %q", got, want)
		}
	})

	t.Run("304 is a terminal media response", func(t *testing.T) {
		calls := 0
		session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			calls++
			if request.Header.Get("If-None-Match") != `"etag-1"` {
				t.Errorf("If-None-Match = %q", request.Header.Get("If-None-Match"))
			}
			w.Header().Set("ETag", `"etag-1"`)
			w.WriteHeader(http.StatusNotModified)
		}), "FANBOXSESSID=conditional-canary")
		response, err := session.OpenMediaWithRequest(context.Background(), "https://downloads.fanbox.cc/media/asset.jpg", fanboxapi.MediaRequest{
			Method: http.MethodGet, IfNoneMatch: `"etag-1"`,
		})
		if err != nil {
			t.Fatalf("OpenMediaWithRequest() error = %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNotModified || calls != 1 {
			t.Fatalf("status = %d calls=%d, want 304 and one request", response.StatusCode, calls)
		}
	})
}

func TestRequestLogsOmitCookieQueryAndBody(t *testing.T) {
	const (
		cookieValue = "session-canary"
		querySecret = "query-canary"
		bodySecret  = "body-canary"
	)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	session, err := fanboxapi.NewSessionWithOptions("FANBOXSESSID="+cookieValue, fanboxapi.SessionOptions{
		Logger: logger,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"secret":"` + bodySecret + `"}`)),
				Request:    request,
			}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := logging.WithOperation(t.Context(), "Post")
	var result struct{}
	if err := session.GetJSON(ctx, fanboxapi.APIBaseURL+"post.info?postId="+querySecret, &result); err != nil {
		t.Fatal(err)
	}
	logged := output.String()
	for _, want := range []string{`"method":"GET"`, `"path":"/post.info"`, `"status":200`, `"sdk_operation":"Post"`} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log missing %s: %s", want, logged)
		}
	}
	for _, secret := range []string{cookieValue, querySecret, bodySecret} {
		if strings.Contains(logged, secret) {
			t.Fatalf("log leaked %q: %s", secret, logged)
		}
	}
}
