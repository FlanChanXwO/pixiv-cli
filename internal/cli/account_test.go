package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountAddListUseRemovePreservesOrder(t *testing.T) {
	authPath, _ := useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add", "--token", "foo=bar; refresh_token=main%2Ftoken", "main"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	info, err := os.Stat(authPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(auth.DefaultAuthFileMode), info.Mode().Perm())

	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Equal(t, "main", store.DefaultAccount)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, "main/token", store.Accounts[0].RefreshToken)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "add", "--token", "other-token", "other"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	store, err = auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 2)
	assert.Equal(t, []string{"main", "other"}, store.Names())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "use", "other"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	store, err = auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	assert.Equal(t, "other", store.DefaultAccount)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	var out accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, "other", out.DefaultAccount)
	require.Len(t, out.Accounts, 2)
	assert.Equal(t, "main", out.Accounts[0].Name)
	assert.Equal(t, "other", out.Accounts[1].Name)
	assert.NotContains(t, stdout.String(), "other-token")
	assert.NotContains(t, stdout.String(), "main/token")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "remove", "other"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	store, err = auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	assert.Equal(t, "main", store.DefaultAccount)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, "main", store.Accounts[0].Name)
}

func TestAccountAddReadsTokenFromStdin(t *testing.T) {
	authPath, _ := useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add", "main"}, strings.NewReader("stdin-token\n"), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	assert.Equal(t, "stdin-token", store.Accounts[0].RefreshToken)
}

func TestAccountAddRejectsCookieWithoutRefreshToken(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add", "--token", "PHPSESSID=web; device_token=device", "main"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), "refresh_token")
}

func TestResolveRefreshTokenPriority(t *testing.T) {
	useTempPaths(t)
	store := auth.AuthStore{
		DefaultAccount: "main",
		Accounts: []auth.Account{
			{Name: "main", RefreshToken: "main-token"},
			{Name: "other", RefreshToken: "other-token"},
		},
	}
	t.Setenv("PIXIV_REFRESH_TOKEN", "env-token")

	token, err := application.ResolveRefreshToken(store, "", "", func() string { return "env-token" })
	require.NoError(t, err)
	assert.Equal(t, "env-token", token)

	token, err = application.ResolveRefreshToken(store, "other", "", func() string { return "env-token" })
	require.NoError(t, err)
	assert.Equal(t, "other-token", token)

	token, err = application.ResolveRefreshToken(store, "other", "flag-token", func() string { return "env-token" })
	require.NoError(t, err)
	assert.Equal(t, "flag-token", token)
}

func TestAccountPromptFlows(t *testing.T) {
	authPath, _ := useTempPaths(t)
	setPromptStub(t, promptStub{
		inputs:   []string{"main"},
		secrets:  []string{"prompt-token"},
		selects:  []string{"main", "main"},
		confirms: []bool{true},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, "main", store.Accounts[0].Name)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "use"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "remove"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	store, err = auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	assert.Empty(t, store.Accounts)
}

func TestAccountRemovePromptCancelKeepsData(t *testing.T) {
	authPath, _ := useTempPaths(t)
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultAccount: "main",
		Accounts:       []auth.Account{{Name: "main", RefreshToken: "main-token"}},
	}))
	setPromptStub(t, promptStub{
		selects:  []string{"main"},
		confirms: []bool{false},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "remove"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, "main", store.Accounts[0].Name)
}

func TestAccountLoginNoOpenStoresProfile(t *testing.T) {
	authPath, _ := useTempPaths(t)
	addr := freeLoopbackAddr(t)

	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/auth/token", r.URL.Path)
		require.Equal(t, pixiv.DefaultUserAgent, r.Header.Get("User-Agent"))
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
		assert.Equal(t, "manual-code", r.Form.Get("code"))
		assert.NotEmpty(t, r.Form.Get("code_verifier"))
		assert.Equal(t, pixiv.DefaultOAuthRedirectURI, r.Form.Get("redirect_uri"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-secret",
			"refresh_token": "refresh-secret",
			"user":          map[string]any{"id": "12345"},
		}))
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()

	calledOpen := false
	restoreOpen := setTestOpenBrowser(t, func(string) error {
		calledOpen = true
		return nil
	})
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "auth", "login", "--addr", addr, "--no-open", "--timeout", "5s", "main"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()

	waitForLoginServer(t, addr)
	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {"manual-code"}})
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	require.Equal(t, 0, run.waitWithin(t, 5*time.Second), stderr.String())
	assert.False(t, calledOpen)

	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Equal(t, "main", store.DefaultAccount)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, "refresh-secret", store.Accounts[0].RefreshToken)
	assert.Equal(t, int64(12345), store.Accounts[0].UserID)
	assert.NotContains(t, stdout.String(), "refresh-secret")
	assert.NotContains(t, stderr.String(), "refresh-secret")
}

func TestAccountLoginBrowserFailureFallsBackToTerminalPrompt(t *testing.T) {
	authPath, _ := useTempPaths(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "terminal-code", r.Form.Get("code"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "prompt-refresh-secret",
			"user":          map[string]any{"id": "24680"},
		}))
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()
	restoreOpen := setTestOpenBrowser(t, func(string) error {
		return errors.New("opener unavailable")
	})
	defer restoreOpen()
	setPromptStub(t, promptStub{
		inputs: []string{"main", "terminal-code"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "login", "--addr", addr, "--timeout", "5s"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, "main", store.Accounts[0].Name)
	assert.Equal(t, "prompt-refresh-secret", store.Accounts[0].RefreshToken)
	assert.Contains(t, stderr.String(), "warning: could not open browser")
}

func setTestOAuthBase(t *testing.T, baseURL string) func() {
	t.Helper()
	return setLoginOAuthBaseForTest(baseURL)
}

func setTestOpenBrowser(t *testing.T, opener func(string) error) func() {
	t.Helper()
	return setOpenBrowserForTest(opener)
}

func setTestCLIClientFactory(t *testing.T, factory func(clientConfig) (cliPixivClient, error)) {
	t.Helper()
	old := newCLIServices
	newCLIServices = func(logger *slog.Logger) application.Services {
		services := bootstrap.NewServices(logger)
		newClient := func(cfg config.RuntimeConfig) (application.ClientBundle, error) {
			client, err := factory(clientConfig{RuntimeConfig: cfg})
			if err != nil {
				return application.ClientBundle{}, err
			}
			return application.ClientBundle{Auth: client, Artwork: client, Download: client}, nil
		}
		services.Artwork.Resolver.NewClient = newClient
		services.Download.Resolver.NewClient = newClient
		services.Download.NewDownloader = func(client application.DownloadClient, cfg config.RuntimeConfig) application.Downloader {
			return download.NewManager(client, logger, cfg.DownloadPath, cfg.FilenameTemplate)
		}
		services.Account.NewClient = func(cfg config.RuntimeConfig) (application.AuthenticatedPixivClient, error) {
			client, err := newClient(cfg)
			if err != nil {
				return nil, err
			}
			return client.Auth, nil
		}
		return services
	}
	t.Cleanup(func() { newCLIServices = old })
}

type promptStub struct {
	inputs   []string
	secrets  []string
	selects  []string
	confirms []bool
}

func setPromptStub(t *testing.T, stub promptStub) {
	t.Helper()
	oldCanPrompt := canPrompt
	oldInput := promptInput
	oldSecret := promptSecret
	oldSelect := promptSelect
	oldConfirm := promptConfirm
	canPrompt = func(app) bool { return true }
	promptInput = func(a app, message, defaultValue string) (string, error) {
		require.NotEmpty(t, stub.inputs, "missing prompt input for %s", message)
		value := stub.inputs[0]
		stub.inputs = stub.inputs[1:]
		return value, nil
	}
	promptSecret = func(a app, message string) (string, error) {
		require.NotEmpty(t, stub.secrets, "missing prompt secret for %s", message)
		value := stub.secrets[0]
		stub.secrets = stub.secrets[1:]
		return value, nil
	}
	promptSelect = func(a app, message string, options []string) (string, error) {
		require.NotEmpty(t, stub.selects, "missing prompt select for %s", message)
		value := stub.selects[0]
		stub.selects = stub.selects[1:]
		return value, nil
	}
	promptConfirm = func(a app, message string, defaultValue bool) (bool, error) {
		require.NotEmpty(t, stub.confirms, "missing prompt confirm for %s", message)
		value := stub.confirms[0]
		stub.confirms = stub.confirms[1:]
		return value, nil
	}
	t.Cleanup(func() {
		canPrompt = oldCanPrompt
		promptInput = oldInput
		promptSecret = oldSecret
		promptSelect = oldSelect
		promptConfirm = oldConfirm
	})
}

func useTempPaths(t *testing.T) (string, string) {
	t.Helper()
	base := filepath.Join(t.TempDir(), "pixiv")
	authPath := filepath.Join(base, "auth.json")
	configPath := filepath.Join(base, "config.toml")
	t.Cleanup(auth.SetAuthFilePathForTest(authPath))
	t.Cleanup(config.SetFilePathForTest(configPath))
	return authPath, configPath
}

type asyncCLIRun struct {
	done     chan int
	mu       sync.Mutex
	received bool
	code     int
}

func startAsyncCLIRun(args []string, in io.Reader, out io.Writer, errOut io.Writer) *asyncCLIRun {
	run := &asyncCLIRun{done: make(chan int, 1)}
	go func() {
		run.done <- Run(args, in, out, errOut)
	}()
	return run
}

func (r *asyncCLIRun) wait() int {
	r.mu.Lock()
	if r.received {
		code := r.code
		r.mu.Unlock()
		return code
	}
	r.mu.Unlock()

	code := <-r.done
	r.mu.Lock()
	if !r.received {
		r.code = code
		r.received = true
	}
	code = r.code
	r.mu.Unlock()
	return code
}

func (r *asyncCLIRun) waitWithin(t *testing.T, timeout time.Duration) int {
	t.Helper()
	r.mu.Lock()
	if r.received {
		code := r.code
		r.mu.Unlock()
		return code
	}
	r.mu.Unlock()

	select {
	case code := <-r.done:
		r.mu.Lock()
		if !r.received {
			r.code = code
			r.received = true
		}
		code = r.code
		r.mu.Unlock()
		return code
	case <-time.After(timeout):
		t.Fatalf("login command did not finish")
		return 1
	}
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func waitForLoginServer(t *testing.T, addr string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return string(body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("login server did not start at %s", addr)
	return ""
}

func loginURLFromPage(t *testing.T, page string) string {
	t.Helper()
	const marker = `href="`
	start := strings.Index(page, marker)
	require.GreaterOrEqual(t, start, 0, page)
	start += len(marker)
	end := strings.Index(page[start:], `"`)
	require.GreaterOrEqual(t, end, 0, page)
	return html.UnescapeString(page[start : start+end])
}

func loginState(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)
	return state
}
