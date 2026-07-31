package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/loginhelper"
	"github.com/stretchr/testify/require"
)

// synchronizedOutput 让 relay goroutine 可以安全写出会话页 URL，测试同时等待它。
type synchronizedOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	writes chan struct{}
}

func newSynchronizedOutput() *synchronizedOutput {
	return &synchronizedOutput{writes: make(chan struct{}, 1)}
}

func (output *synchronizedOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	n, err := output.buffer.Write(data)
	output.mu.Unlock()
	select {
	case output.writes <- struct{}{}:
	default:
	}
	return n, err
}

func (output *synchronizedOutput) waitForSessionURL(t *testing.T) string {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		output.mu.Lock()
		current := output.buffer.String()
		output.mu.Unlock()
		const marker = "Open remote Pixiv login session:\n"
		if index := strings.Index(current, marker); index >= 0 {
			after := current[index+len(marker):]
			line, _, _ := strings.Cut(after, "\n")
			return line
		}
		select {
		case <-output.writes:
		case <-deadline.C:
			t.Fatal("handoff relay did not print a session URL")
		}
	}
}

// remoteLoginDeepLinkFromSessionURL 断言一次性 session URL 不渲染项目中间页，
// 而是直接把 browser 交给当前会话的 desktop handler。
func remoteLoginDeepLinkFromSessionURL(t *testing.T, sessionURL string) string {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Get(sessionURL)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, response.StatusCode)
	require.NotContains(t, string(body), "Desktop handoff")
	require.NotContains(t, string(body), "Manual completion")
	location := response.Header.Get("Location")
	require.NotEmpty(t, location)
	return location
}

func postHandoffCallback(t *testing.T, endpoint, proof, callback string) *http.Response {
	t.Helper()
	body, err := json.Marshal(relayLoginCallbackRequest{CallbackURL: callback, Proof: proof})
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	return response
}

// TestRemoteLoginHandoffAcceptsPixivCallbackWithoutState 覆盖一次性远程会话的
// 最短可用路径。官方 pixiv:// callback 可不含 state；此时仍由 server 中同一次
// LoginSession 的 PKCE verifier 绑定，不能为 remote relay 另加 state 要求。
func TestRemoteLoginHandoffAcceptsPixivCallbackWithoutState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	legacyDevicesPath := filepath.Join(os.Getenv("HOME"), ".pixiv-cli", "remote-devices.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyDevicesPath), 0o700))
	legacyDevices := []byte("legacy device data that must remain untouched")
	require.NoError(t, os.WriteFile(legacyDevicesPath, legacyDevices, 0o600))
	addr := freeLoopbackAddr(t)
	const callback = "pixiv://account/login?code=handoff-one-time-code"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	contextWaiterExited := make(chan struct{})

	output := newSynchronizedOutput()
	a := app{errOut: output}
	type outcome struct {
		code    string
		notify  func(bool)
		cleanup func()
		err     error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		code, notify, cleanup, err := a.waitForHandoffRelayLoginCode(ctx, loginRelayServerOptions{
			PublicURL:           "http://" + addr,
			ListenAddr:          addr,
			contextWaiterExited: func() { close(contextWaiterExited) },
		}, func(raw string) bool { return raw == callback }, "https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=handoff-challenge&state=handoff")
		resultCh <- outcome{code: code, notify: notify, cleanup: cleanup, err: err}
	}()

	waitForLoginServer(t, addr)
	sessionURL := output.waitForSessionURL(t)
	deepLink := remoteLoginDeepLinkFromSessionURL(t, sessionURL)
	start, err := loginhelper.ParseRemoteLoginLink(deepLink)
	require.NoError(t, err)
	manualRequest, err := http.NewRequest(http.MethodPost, "http://"+addr+"/manual/"+start.SessionID, nil)
	require.NoError(t, err)
	manualResponse, err := http.DefaultClient.Do(manualRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, manualResponse.StatusCode)
	require.NoError(t, manualResponse.Body.Close())
	loginURL, err := loginhelper.StartRemoteLogin(context.Background(), start)
	require.NoError(t, err)
	require.Equal(t, "https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=handoff-challenge&state=handoff", loginURL)

	remote, err := loginhelper.ForwardActiveRemoteLoginCallback(context.Background(), callback)
	require.NoError(t, err)
	t.Cleanup(remote.Abort)
	require.NotEmpty(t, remote.ResultURL)
	actualLegacyDevices, err := os.ReadFile(legacyDevicesPath)
	require.NoError(t, err)
	require.Equal(t, legacyDevices, actualLegacyDevices, "handoff must not read or rewrite obsolete device state")
	_, err = os.Stat(filepath.Join(os.Getenv("HOME"), ".pixiv-cli", "remote-login-session.json"))
	require.ErrorIs(t, err, os.ErrNotExist, "successful remote callback must consume local handoff state")

	replay := postHandoffCallback(t, "http://"+addr+"/callback/"+start.SessionID, start.Proof, callback)
	require.Equal(t, http.StatusConflict, replay.StatusCode)
	require.NoError(t, replay.Body.Close())

	pageCh := make(chan error, 1)
	go func() {
		response, err := http.Get(remote.ResultURL)
		if err != nil {
			pageCh <- err
			return
		}
		defer response.Body.Close()
		_, err = io.ReadAll(response.Body)
		pageCh <- err
	}()

	select {
	case result := <-resultCh:
		require.NoError(t, result.err)
		require.Equal(t, callback, result.code)
		select {
		case <-contextWaiterExited:
		case <-time.After(5 * time.Second):
			t.Fatal("handoff relay context watcher remained after callback submission")
		}
		result.notify(true)
		result.cleanup()
	case <-time.After(5 * time.Second):
		t.Fatal("handoff relay did not deliver callback")
	}
	require.NoError(t, remote.Complete())
	require.NoError(t, <-pageCh)
}

// PublicURL 同时决定给用户的 session URL、手动提交地址与 desktop deep link 的
// relay origin；三者必须从同一规范形式生成，避免反向代理路径出现不一致。
func TestRemoteLoginHandoffCanonicalizesPublicURLEverywhere(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	addr := freeLoopbackAddr(t)
	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := newSynchronizedOutput()
	a := app{errOut: output}
	type outcome struct {
		cleanup func()
		err     error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		_, _, cleanup, err := a.waitForHandoffRelayLoginCode(ctx, loginRelayServerOptions{
			PublicURL:  "HTTP://LOCALHOST:" + port + "//remote//login///",
			ListenAddr: addr,
		}, func(string) bool { return false }, "https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=handoff-challenge&state=handoff")
		resultCh <- outcome{cleanup: cleanup, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case result := <-resultCh:
			if result.cleanup != nil {
				result.cleanup()
			}
		case <-time.After(5 * time.Second):
			t.Fatal("handoff relay did not stop after cancellation")
		}
	})

	waitForLoginServer(t, addr)
	sessionURL := output.waitForSessionURL(t)
	require.Regexp(t, `^http://localhost:`+port+`/remote/login/session/`, sessionURL)
	parsedSessionURL, err := url.Parse(sessionURL)
	require.NoError(t, err)
	// 测试直连 listener；生产环境的 /remote/login 前缀由 reverse proxy 转发。
	directSessionURL := "http://" + addr + "/session/" + path.Base(parsedSessionURL.Path)
	deepLink := remoteLoginDeepLinkFromSessionURL(t, directSessionURL)
	start, err := loginhelper.ParseRemoteLoginLink(deepLink)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:"+port+"/remote/login", start.Origin)
}

func TestRemoteLoginHandoffRejectsMismatchedCapability(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	addr := freeLoopbackAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := newSynchronizedOutput()
	a := app{errOut: output}
	type outcome struct {
		cleanup func()
		err     error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		_, _, cleanup, err := a.waitForHandoffRelayLoginCode(ctx, loginRelayServerOptions{
			PublicURL:  "http://" + addr,
			ListenAddr: addr,
		}, func(string) bool { return true }, "https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=handoff-challenge&state=handoff")
		resultCh <- outcome{cleanup: cleanup, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case result := <-resultCh:
			if result.cleanup != nil {
				result.cleanup()
			}
		case <-time.After(5 * time.Second):
			t.Fatal("handoff relay did not stop after cancellation")
		}
	})

	waitForLoginServer(t, addr)
	start, err := loginhelper.ParseRemoteLoginLink(remoteLoginDeepLinkFromSessionURL(t, output.waitForSessionURL(t)))
	require.NoError(t, err)
	start.Proof = "different-session-capability"
	_, err = loginhelper.StartRemoteLogin(context.Background(), start)
	require.EqualError(t, err, "remote Pixiv login relay rejected login handoff")
	_, err = os.Stat(filepath.Join(os.Getenv("HOME"), ".pixiv-cli", "remote-login-session.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRemoteLoginHandoffRejectsCallbackBeforeDesktopStartAndWithWrongState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	addr := freeLoopbackAddr(t)
	const callback = "pixiv://account/login?code=must-not-be-accepted-before-start"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	output := newSynchronizedOutput()
	a := app{errOut: output}
	type outcome struct {
		cleanup func()
		err     error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		_, _, cleanup, err := a.waitForHandoffRelayLoginCode(ctx, loginRelayServerOptions{
			PublicURL:  "http://" + addr,
			ListenAddr: addr,
		}, func(raw string) bool {
			return raw == callback || raw == callback+"&state=matching-state"
		}, "https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=handoff-challenge&state=handoff")
		resultCh <- outcome{cleanup: cleanup, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case result := <-resultCh:
			if result.cleanup != nil {
				result.cleanup()
			}
		case <-time.After(5 * time.Second):
			t.Fatal("handoff relay did not stop after cancellation")
		}
	})

	waitForLoginServer(t, addr)
	start, err := loginhelper.ParseRemoteLoginLink(remoteLoginDeepLinkFromSessionURL(t, output.waitForSessionURL(t)))
	require.NoError(t, err)
	response := postHandoffCallback(t, "http://"+addr+"/callback/"+start.SessionID, start.Proof, callback)
	require.Equal(t, http.StatusConflict, response.StatusCode)
	require.NoError(t, response.Body.Close())

	_, err = loginhelper.StartRemoteLogin(context.Background(), start)
	require.NoError(t, err)
	wrongState := postHandoffCallback(t, "http://"+addr+"/callback/"+start.SessionID, start.Proof, callback+"&state=other-session")
	require.Equal(t, http.StatusBadRequest, wrongState.StatusCode)
	require.NoError(t, wrongState.Body.Close())
}

func TestHandoffRelayContextWaiterStopsWhenServerFails(t *testing.T) {
	contextWaiterExited := make(chan struct{})
	a := app{errOut: io.Discard}
	_, _, cleanup, err := a.waitForHandoffRelayLoginCode(context.Background(), loginRelayServerOptions{
		PublicURL:           "http://relay.example",
		ListenAddr:          "127.0.0.1:1",
		contextWaiterExited: func() { close(contextWaiterExited) },
		listen: func(string, string) (net.Listener, error) {
			return failingHandoffListener{}, nil
		},
	}, func(string) bool { return false }, "https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=handoff-challenge&state=handoff")
	require.EqualError(t, err, "remote login relay server failed; verify its listener and TLS configuration")
	cleanup()
	select {
	case <-contextWaiterExited:
	case <-time.After(5 * time.Second):
		t.Fatal("handoff relay context watcher remained after relay server failure")
	}
}

func TestHandoffRelayContextWaiterStopsWhenCancelled(t *testing.T) {
	addr := freeLoopbackAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	contextWaiterExited := make(chan struct{})
	a := app{errOut: io.Discard}
	type outcome struct {
		cleanup func()
		err     error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		_, _, cleanup, err := a.waitForHandoffRelayLoginCode(ctx, loginRelayServerOptions{
			PublicURL:           "http://" + addr,
			ListenAddr:          addr,
			contextWaiterExited: func() { close(contextWaiterExited) },
		}, func(string) bool { return false }, "https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=handoff-challenge&state=handoff")
		resultCh <- outcome{cleanup: cleanup, err: err}
	}()

	waitForLoginServer(t, addr)
	cancel()
	select {
	case result := <-resultCh:
		require.ErrorIs(t, result.err, context.Canceled)
		result.cleanup()
	case <-time.After(5 * time.Second):
		t.Fatal("handoff relay did not stop after cancellation")
	}
	select {
	case <-contextWaiterExited:
	case <-time.After(5 * time.Second):
		t.Fatal("handoff relay context watcher remained after cancellation")
	}
}

func TestRemoteLoginHandlerReportsCleanupFailureAfterBrowserOpenFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	relay := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "/start/session", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"authorization_url":"https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=challenge&state=state"}`)
	}))
	t.Cleanup(relay.Close)

	originalClear := clearRemoteLoginHandoffForHandler
	clearRemoteLoginHandoffForHandler = func(loginhelper.RemoteLoginStart) error {
		return errors.New("forced local cleanup failure")
	}
	t.Cleanup(func() { clearRemoteLoginHandoffForHandler = originalClear })
	t.Cleanup(setOpenBrowserForTest(func(string) error { return errors.New("forced browser failure") }))

	deepLink := (&url.URL{
		Scheme: "pixiv",
		Host:   "account",
		Path:   "/remote-login",
		RawQuery: url.Values{
			"origin":  {relay.URL},
			"session": {"session"},
			"access":  {"proof"},
		}.Encode(),
	}).String()
	command := (app{errOut: io.Discard}).newAccountURLCallbackCommand()
	command.SetArgs([]string{deepLink})
	err := command.ExecuteContext(context.Background())
	require.EqualError(t, err, "could not clear remote Pixiv login handoff after browser launch failed")
}

type failingHandoffListener struct{}

func (failingHandoffListener) Accept() (net.Conn, error) {
	return nil, errors.New("forced listener failure")
}
func (failingHandoffListener) Close() error { return nil }
func (failingHandoffListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}
}
