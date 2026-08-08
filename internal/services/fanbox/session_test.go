package fanbox

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestSession 启动一个信任自签证书的 httptest TLS server，并把任意 fanbox/媒体
// 主机名的连接重定向到该 server，从而用标准 net/http 全栈测试 allowlist 行为。
func newTestSession(t *testing.T, handler http.Handler, cookie string) *Session {
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
	session, err := NewSessionWithHTTPClient(cookie, &http.Client{Transport: transport})
	if err != nil {
		t.Fatalf("NewSessionWithHTTPClient() error = %v", err)
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

func TestCurrentUserParsesHomepageMetadata(t *testing.T) {
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Host != "www.fanbox.cc" || request.URL.Path != "/" {
			t.Errorf("request = %q host=%q", request.URL, request.Host)
		}
		if got := request.Header.Get("Cookie"); got != "FANBOXSESSID=identity-canary" {
			t.Errorf("Cookie = %q", got)
		}
		if got := request.Header.Get("Origin"); got != "https://www.fanbox.cc" {
			t.Errorf("Origin = %q", got)
		}
		_, _ = io.WriteString(w, `<meta name="metadata" content='{"context":{"user":{"userId":"800","name":"verified","creatorId":"artist","creatorStatus":"explicit","isCreator":true}}}'>`)
	}), "FANBOXSESSID=identity-canary")

	identity, err := session.CurrentUser(context.Background())
	if err != nil {
		t.Fatalf("CurrentUser() error = %v", err)
	}
	want := Identity{UserID: 800, DisplayName: "verified", CreatorID: "artist", CreatorStatus: "explicit", IsCreator: true}
	if identity != want {
		t.Fatalf("identity = %+v, want %+v", identity, want)
	}
}

func TestCurrentUserRejectsMissingMetadata(t *testing.T) {
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(w, "<html><head><title>fanbox</title></head></html>")
	}), "FANBOXSESSID=identity-canary")

	if _, err := session.CurrentUser(context.Background()); err == nil {
		t.Fatal("CurrentUser() unexpectedly succeeded without metadata")
	}
}

func TestMediaOpenSendsCookieOnlyToDownloadsHost(t *testing.T) {
	var seen []string
	session := newTestSession(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Host+":"+request.Header.Get("Cookie"))
		got := request.Header.Get("Cookie")
		if request.Host == "downloads.fanbox.cc" && got != "FANBOXSESSID=media-canary" {
			t.Errorf("downloads Cookie = %q", got)
		}
		if request.Host != "downloads.fanbox.cc" && got != "" {
			t.Errorf("redirected media Cookie = %q", got)
		}
		if request.Host == "downloads.fanbox.cc" {
			w.Header().Set("Location", "https://i.pximg.net/media/asset.jpg")
			w.WriteHeader(http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "media")
	}), "FANBOXSESSID=media-canary")

	response, err := session.OpenMedia(context.Background(), "https://downloads.fanbox.cc/media/asset.jpg")
	if err != nil {
		t.Fatalf("OpenMedia() error = %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read/close media error = %v / %v", readErr, closeErr)
	}
	if string(body) != "media" {
		t.Fatalf("media body = %q", body)
	}
	if got, want := strings.Join(seen, ","), "downloads.fanbox.cc:FANBOXSESSID=media-canary,i.pximg.net:"; got != want {
		t.Fatalf("requests = %q, want %q", got, want)
	}
}

func TestAPIRedirectDropsCookie(t *testing.T) {
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

	if _, err := session.Post(context.Background(), "post-1"); err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	want := "api.fanbox.cc/post.info:FANBOXSESSID=redirect-canary,www.fanbox.cc/post-info:"
	if got := strings.Join(seen, ","); got != want {
		t.Fatalf("requests = %q, want %q", got, want)
	}
}

func TestStatusClassification(t *testing.T) {
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

	cases := []struct {
		path string
		want error
	}{
		{"/unauthorized", ErrNotAuthenticated},
		{"/forbidden", ErrForbidden},
		{"/challenge-body", ErrChallenge},
		{"/challenge-html", ErrChallenge},
		{"/challenge-header", ErrChallenge},
		{"/notfound", nil},
	}
	for _, test := range cases {
		t.Run(test.path, func(t *testing.T) {
			_, err := session.Home(context.Background(), "https://api.fanbox.cc"+test.path)
			if test.want == nil {
				if err == nil {
					t.Fatal("endpoint unexpectedly succeeded")
				}
				if errors.Is(err, ErrForbidden) || errors.Is(err, ErrChallenge) || errors.Is(err, ErrNotAuthenticated) {
					t.Fatalf("generic status was misclassified: %v", err)
				}
				return
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestNewSessionUsesFirefoxTransportAndRejectsImplicitTransport(t *testing.T) {
	session, err := NewSession("FANBOXSESSID=tls-canary")
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	defer session.CloseIdleConnections()
	if _, ok := session.httpClient.Transport.(*firefoxTransport); !ok {
		t.Fatalf("production transport = %T, want *firefoxTransport", session.httpClient.Transport)
	}
	if _, err := NewSessionWithHTTPClient("FANBOXSESSID=tls-canary", &http.Client{}); err == nil {
		t.Fatal("NewSessionWithHTTPClient() accepted implicit standard transport")
	}
	if _, err := NewSession("FANBOXSESSID=tls-canary", WithHTTPClient(&http.Client{})); err == nil {
		t.Fatal("NewSession() with implicit client accepted implicit standard transport")
	}
}

func TestSessionUsesCustomUserAgent(t *testing.T) {
	session, err := NewSessionWithOptions("FANBOXSESSID=ua-canary", SessionOptions{
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
	response, err := session.Home(context.Background(), "https://api.fanbox.cc/post.info?postId=1")
	if err == nil || response.Posts != nil {
		t.Fatalf("Home() = %+v, err=%v; expected decode failure after header assertion", response, err)
	}
}

func TestSessionRejectsUnsafeURLsBeforeTransport(t *testing.T) {
	called := false
	session, err := NewSessionWithHTTPClient("FANBOXSESSID=url-canary", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		"http://downloads.fanbox.cc/media.jpg",
		"https://user@downloads.fanbox.cc/media.jpg",
		"https://example.invalid/media.jpg",
	} {
		_, err := session.OpenMedia(context.Background(), target)
		if err == nil {
			t.Fatalf("OpenMedia(%q) unexpectedly succeeded", target)
		}
		if strings.Contains(err.Error(), "url-canary") {
			t.Fatalf("error disclosed cookie: %v", err)
		}
	}
	if called {
		t.Fatal("transport was called for an unsafe URL")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
