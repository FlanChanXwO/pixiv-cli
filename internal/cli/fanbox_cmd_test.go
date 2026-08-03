package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/authdb"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func fanboxHomeMetadataBody(userID int64, name string) string {
	return `<html><head><meta name="metadata" content='{"context":{"user":{"userId":` + fmt.Sprintf("%d", userID) + `,"name":"` + name + `"}}}'></head></html>`
}

// fanboxSessionOKRoundTripper 只服务 CurrentUser 的首页元数据。
func fanboxSessionOKRoundTripper(userID int64, name string) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/" {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/html"}},
				Body:       io.NopCloser(strings.NewReader(fanboxHomeMetadataBody(userID, name))),
			}, nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
}

// fanboxPostInfoRoundTripper 服务单帖 post.info 与资源打开。
func fanboxPostInfoRoundTripper(postID, title, creatorID string) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/v1/post.info":
			body := `{"body":{"post":{"id":"` + postID + `","title":"` + title + `","publishedDatetime":"2024-06-01T10:00:00Z","creatorId":"` + creatorID + `","feeRequired":0,"isRestricted":false,"isPinned":false,"body":{"text":"caption","images":[],"files":[]}}}}`
			return jsonResponse(body), nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
	})
}

func fanboxSessionFailRoundTripper() http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
}

// fanboxTestHarness 建立隔离 HOME + 临时 FANBOX 鉴权数据库的服务，客户端打开走
// 注入的 RoundTripper，绝不拨号真实网络。
type fanboxTestHarness struct {
	db      *authdb.DB
	service *fanboxapp.Service
}

func newFanboxTestHarness(t *testing.T, rt http.RoundTripper) *fanboxTestHarness {
	t.Helper()
	useTempPaths(t)
	appDataDir := filepath.Join(os.Getenv("HOME"), constants.AppDataDirName)
	db, err := authdb.Open(appDataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	service := fanboxapp.New(db, appDataDir)
	service.OpenSessionFunc = func(value string) (*fanbox.Client, error) {
		return fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: value}, fanbox.Options{HTTPClient: &http.Client{Transport: rt}})
	}
	service.OpenClientFunc = func(context.Context) (*fanbox.Client, error) {
		return fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: "stored-session-canary"}, fanbox.Options{HTTPClient: &http.Client{Transport: rt}})
	}
	return &fanboxTestHarness{db: db, service: service}
}

func (h *fanboxTestHarness) install(t *testing.T) {
	t.Helper()
	old := newCLIServices
	newCLIServices = func() application.Services {
		return application.Services{Fanbox: h.service}
	}
	t.Cleanup(func() { newCLIServices = old })
}

func TestFanboxAuthImportStdinHappyPath(t *testing.T) {
	h := newFanboxTestHarness(t, fanboxSessionOKRoundTripper(42, "tester"))
	h.install(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "fanbox", "auth", "import", "--stdin"}, strings.NewReader("session-canary-value\n"), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	output := stdout.String()
	assert.Contains(t, output, "imported uid:42")
	assert.Contains(t, output, "display:tester")
	assert.NotContains(t, output, "session-canary-value")
	assert.NotContains(t, stderr.String(), "session-canary-value")
	accounts, err := h.db.ListFanbox(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	assert.Equal(t, int64(42), accounts[0].UserID)
	assert.Equal(t, "session-canary-value", string(accounts[0].SessionID))
}

func TestFanboxAuthImportStdinValidationFailureCreatesNoRecord(t *testing.T) {
	h := newFanboxTestHarness(t, fanboxSessionFailRoundTripper())
	h.install(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "fanbox", "auth", "import", "--stdin"}, strings.NewReader("invalid-session-value\n"), &stdout, &stderr)
	require.NotEqual(t, 0, code)
	assert.Empty(t, stdout.String())
	assert.NotContains(t, stdout.String(), "invalid-session-value")
	assert.NotContains(t, stderr.String(), "invalid-session-value")
	accounts, err := h.db.ListFanbox(context.Background())
	require.NoError(t, err)
	assert.Empty(t, accounts)
}

func TestFanboxAuthImportRejectsStdinWithFromBrowser(t *testing.T) {
	h := newFanboxTestHarness(t, fanboxSessionOKRoundTripper(42, "tester"))
	h.install(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "fanbox", "auth", "import", "--stdin", "--from-browser", "chrome"}, strings.NewReader("value\n"), &stdout, &stderr)
	require.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "exactly one")
	assert.Empty(t, stdout.String())
}

func TestFanboxAuthImportFromBrowserUnavailable(t *testing.T) {
	h := newFanboxTestHarness(t, fanboxSessionOKRoundTripper(42, "tester"))
	h.install(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "fanbox", "auth", "import", "--from-browser", "chrome"}, strings.NewReader(""), &stdout, &stderr)
	require.NotEqual(t, 0, code)
	assert.Contains(t, stderr.String(), "browser session import is not available")
	assert.Empty(t, stdout.String())
}

type fakeFanboxBrowserProvider struct{ value string }

func (f fakeFanboxBrowserProvider) ReadSession(context.Context, string, string) (string, error) {
	return f.value, nil
}

func TestFanboxAuthImportFromBrowserInjectedProvider(t *testing.T) {
	h := newFanboxTestHarness(t, fanboxSessionOKRoundTripper(7, "browser-user"))
	h.install(t)
	old := fanboxBrowserSessionReader
	fanboxBrowserSessionReader = fakeFanboxBrowserProvider{value: "browser-session-canary"}
	t.Cleanup(func() { fanboxBrowserSessionReader = old })
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "fanbox", "auth", "import", "--from-browser", "chrome", "--profile", "p1", "--default"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "imported uid:7")
	assert.NotContains(t, stdout.String(), "browser-session-canary")
	accounts, err := h.db.ListFanbox(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	assert.Equal(t, "browser-session-canary", string(accounts[0].SessionID))
}

func TestFanboxAuthListUseRemoveStatus(t *testing.T) {
	h := newFanboxTestHarness(t, fanboxSessionOKRoundTripper(42, "tester"))
	h.install(t)
	_, err := h.service.ImportSession(context.Background(), "session-abc", true)
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "fanbox", "auth", "list"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "* uid:42")
	assert.NotContains(t, stdout.String(), "session-abc")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "fanbox", "auth", "use", "--auto"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "default uid: auto")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "fanbox", "auth", "use", "42"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "default uid: 42")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "fanbox", "auth", "status"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "uid:42")
	assert.Contains(t, stdout.String(), "default:yes")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "fanbox", "auth", "status", "42"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "uid:42")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "fanbox", "auth", "remove", "42", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "account uid:42 removed")

	accounts, err := h.db.ListFanbox(context.Background())
	require.NoError(t, err)
	assert.Empty(t, accounts)
}

func TestFanboxAuthOutputIsolation(t *testing.T) {
	h := newFanboxTestHarness(t, fanboxSessionOKRoundTripper(42, "tester"))
	h.install(t)
	_, err := h.service.ImportSession(context.Background(), "SESSION-CANARY-987", true)
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "fanbox", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), `"user_id": 42`)
	assert.NotContains(t, stdout.String(), "SESSION-CANARY-987")
	assert.NotContains(t, stderr.String(), "SESSION-CANARY-987")
}

func TestFanboxPostNDJSONOutputIsolation(t *testing.T) {
	h := newFanboxTestHarness(t, fanboxPostInfoRoundTripper("p1", "hello", "pixiv"))
	h.install(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "fanbox", "post", "p1", "--ndjson"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), `"id":"p1"`)
	assert.Contains(t, stdout.String(), `"title":"hello"`)
	assert.NotContains(t, stdout.String(), "stored-session-canary")
	assert.NotContains(t, stderr.String(), "stored-session-canary")
}

func TestFanboxPostJSONSingle(t *testing.T) {
	h := newFanboxTestHarness(t, fanboxPostInfoRoundTripper("p1", "hello", "pixiv"))
	h.install(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "fanbox", "post", "p1", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), `"id": "p1"`)
	assert.Contains(t, stdout.String(), `"is_restricted": false`)
	assert.NotContains(t, stdout.String(), "stored-session-canary")
}
