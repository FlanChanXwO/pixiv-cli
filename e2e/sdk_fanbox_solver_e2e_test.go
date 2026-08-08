package e2e

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

const (
	fanboxSolverE2EEnabledEnv = "FANBOX_SOLVER_E2E"
	fanboxSolverURLValueEnv   = "FANBOX_SOLVER_URL"
	fanboxSolverProxyEnv      = "FANBOX_SOLVER_PROXY"
)

// TestRealFanboxSolverProtocolAcceptance 验证 public FANBOX SDK 与真实 FlareSolverr
// service 的协议链路；native request 保持 synthetic。dummy session 只进入注入的
// native transport，solver 只能收到生产路径发出的匿名首页请求。
func TestRealFanboxSolverProtocolAcceptance(t *testing.T) {
	if os.Getenv(fanboxSolverE2EEnabledEnv) != "1" {
		t.Skip("set FANBOX_SOLVER_E2E=1 to run the real FlareSolverr acceptance")
	}

	solverURL := strings.TrimSpace(os.Getenv(fanboxSolverURLValueEnv))
	parsedSolverURL, err := url.Parse(solverURL)
	if err != nil || (parsedSolverURL.Scheme != "http" && parsedSolverURL.Scheme != "https") || parsedSolverURL.Host == "" || parsedSolverURL.User != nil {
		t.Fatalf("%s must be an absolute HTTP(S) solver URL without userinfo", fanboxSolverURLValueEnv)
	}

	var nativeCalls atomic.Int32
	native := solverAcceptanceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		call := nativeCalls.Add(1)
		if request.URL.Host != "api.fanbox.cc" || request.URL.Path != "/post.info" {
			t.Fatalf("native request target = %s", request.URL)
		}
		if call == 1 {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Cf-Mitigated": {"challenge"}},
				Body:       io.NopCloser(strings.NewReader("synthetic challenge")),
			}, nil
		}
		if call != 2 {
			t.Fatalf("native request count exceeded one replay: %d", call)
		}
		if request.Header.Get("User-Agent") == "" {
			t.Fatal("native replay did not receive a solver User-Agent")
		}
		if cookie := request.Header.Get("Cookie"); !strings.Contains(cookie, "FANBOXSESSID=dummy-session") || !strings.Contains(cookie, "cf_clearance=") {
			t.Fatal("native replay did not receive the session and solver clearance")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"body":{"post":{"id":"post-1","title":"replayed","publishedDatetime":"2024-01-01T00:00:00Z"}}}`)),
		}, nil
	})

	options := fanboxsdk.Options{
		HTTPClient: &http.Client{Transport: native},
		FlareSolverr: &fanboxsdk.FlareSolverrOptions{
			URL:      solverURL,
			ProxyURL: strings.TrimSpace(os.Getenv(fanboxSolverProxyEnv)),
		},
	}
	client, err := fanboxsdk.OpenWith(fanboxsdk.SessionCredentials{FANBOXSESSID: "dummy-session"}, options)
	if err != nil {
		t.Fatalf("fanbox.Open: %v", err)
	}
	defer client.CloseIdleConnections()

	post, err := client.Post(t.Context(), fanboxsdk.PostRequest{PostID: "post-1"})
	if err != nil {
		t.Fatalf("fanbox.Post: %v", err)
	}
	if post.ID != "post-1" || post.Title != "replayed" {
		t.Fatalf("fanbox.Post = id %q title %q", post.ID, post.Title)
	}
	if got := nativeCalls.Load(); got != 2 {
		t.Fatalf("native request count = %d, want initial challenge plus one replay", got)
	}
}

type solverAcceptanceRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn solverAcceptanceRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
