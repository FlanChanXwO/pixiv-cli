package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	internalpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	publicpixiv "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loginOAuthSuccessHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/auth/token" {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "refresh_token": "rotated", "user": map[string]any{"id": 456, "name": "login-user"}})
}

func TestAccountAddListUseRemovePreservesOrder(t *testing.T) {
	authPath, _ := useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"main/token":  {userID: 111, username: "alice"},
		"other-token": {userID: 222, username: "bob"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add", "--token", "main/token"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	info, err := os.Stat(authPath)
	require.NoError(t, err)
	assertPrivateFileMode(t, info.Mode().Perm(), auth.DefaultAuthFileMode)

	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Equal(t, int64(111), store.DefaultUserID)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(111), store.Accounts[0].UserID)
	assert.Equal(t, "alice", store.Accounts[0].Username)
	assert.Equal(t, "main/token", store.Accounts[0].RefreshToken)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "add", "--token", "other-token"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	store, err = auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 2)
	assert.Equal(t, []int64{111, 222}, store.UserIDs())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "use", "222"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	store, err = auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	assert.Equal(t, int64(222), store.DefaultUserID)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	var out accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, int64(222), out.DefaultUserID)
	require.Len(t, out.Accounts, 2)
	assert.Equal(t, int64(111), out.Accounts[0].UserID)
	assert.Equal(t, "alice", out.Accounts[0].Username)
	assert.Equal(t, int64(222), out.Accounts[1].UserID)
	assert.Equal(t, "bob", out.Accounts[1].Username)
	assert.NotContains(t, stdout.String(), "other-token")
	assert.NotContains(t, stdout.String(), "main/token")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "remove", "222"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	store, err = auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	assert.Equal(t, int64(111), store.DefaultUserID)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(111), store.Accounts[0].UserID)
}

func TestAccountAddReadsTokenFromStdin(t *testing.T) {
	authPath, _ := useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"stdin-token": {userID: 333, username: "stdin-user"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add"}, strings.NewReader("stdin-token\n"), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	assert.Equal(t, int64(333), store.Accounts[0].UserID)
	assert.Equal(t, "stdin-user", store.Accounts[0].Username)
	assert.Equal(t, "stdin-token", store.Accounts[0].RefreshToken)
}

func TestAccountAddProxyFlagOverridesEnvAndConfig(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n")))
	t.Setenv("https_proxy", "http://env-proxy")

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-token",
				"refresh_token": "input-token",
				"user":          map[string]any{"id": 444, "name": "proxy-user"},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	proxy := newTestForwardProxy(t)
	setTestPublicSDKFactory(t, api.URL, api.URL, api.URL, publicpixiv.ResourcePolicy{}, func(request application.SDKClientRequest) {
		require.NotNil(t, request.HTTPSProxyOverride)
		assert.Equal(t, proxy.URL, *request.HTTPSProxyOverride)
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add", "--token", "input-token", "--proxy", proxy.URL}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.NotZero(t, proxy.Requests())
}

func TestAccountAddNoProxyFlagClearsEnvAndConfig(t *testing.T) {
	_, configPath := useTempPaths(t)
	proxy := newTestForwardProxy(t)
	require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \""+proxy.URL+"\"\n")))
	t.Setenv("https_proxy", proxy.URL)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-token",
				"refresh_token": "input-token",
				"user":          map[string]any{"id": 445, "name": "no-proxy-user"},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	setTestPublicSDKFactory(t, api.URL, api.URL, api.URL, publicpixiv.ResourcePolicy{}, func(request application.SDKClientRequest) {
		require.NotNil(t, request.HTTPSProxyOverride)
		assert.Empty(t, *request.HTTPSProxyOverride)
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add", "--token", "input-token", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Zero(t, proxy.Requests())
}

func TestAccountCheckNoProxyFlagClearsEnvAndConfig(t *testing.T) {
	authPath, configPath := useTempPaths(t)
	proxy := newTestForwardProxy(t)
	require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \""+proxy.URL+"\"\n")))
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 444,
		Accounts:      []auth.Account{{UserID: 444, RefreshToken: "check-token"}},
	}))
	t.Setenv("https_proxy", proxy.URL)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-token",
				"refresh_token": "check-token",
				"user":          map[string]any{"id": 444, "name": "check-user"},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	setTestPublicSDKFactory(t, api.URL, api.URL, api.URL, publicpixiv.ResourcePolicy{}, func(request application.SDKClientRequest) {
		require.NotNil(t, request.HTTPSProxyOverride)
		assert.Empty(t, *request.HTTPSProxyOverride)
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "check", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Zero(t, proxy.Requests())
}

func TestAccountCheckUsesEnvironmentTokenWithoutChangingDefaultAccount(t *testing.T) {
	authPath, _ := useTempPaths(t)
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 444,
		Accounts:      []auth.Account{{UserID: 444, Username: "stored", RefreshToken: "stored-token"}},
	}))
	t.Setenv("PIXIV_REFRESH_TOKEN", "environment-token")

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" {
			http.NotFound(w, r)
			return
		}
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "environment-token", r.Form.Get("refresh_token"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-token",
			"refresh_token": "rotated-environment-token",
			"user":          map[string]any{"id": 555, "name": "environment-user"},
		}))
	}))
	defer api.Close()
	setTestPublicSDKFactory(t, api.URL, api.URL, api.URL, publicpixiv.ResourcePolicy{}, nil)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "check", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var out accountOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, accountOut{UserID: 555, Username: "environment-user", HasToken: true}, out)

	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	assert.Equal(t, int64(444), store.DefaultUserID)
	require.Equal(t, []auth.Account{{UserID: 444, Username: "stored", RefreshToken: "stored-token"}}, store.Accounts)
}

func TestAccountCheckProxyFlagOverridesEnvAndConfig(t *testing.T) {
	authPath, configPath := useTempPaths(t)
	proxy := newTestForwardProxy(t)
	require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \""+proxy.URL+"\"\n")))
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 446,
		Accounts:      []auth.Account{{UserID: 446, RefreshToken: "check-token"}},
	}))
	t.Setenv("https_proxy", proxy.URL)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-token",
				"refresh_token": "check-token",
				"user":          map[string]any{"id": 446, "name": "check-proxy-user"},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	setTestPublicSDKFactory(t, api.URL, api.URL, api.URL, publicpixiv.ResourcePolicy{}, func(request application.SDKClientRequest) {
		require.NotNil(t, request.HTTPSProxyOverride)
		assert.Equal(t, proxy.URL, *request.HTTPSProxyOverride)
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "check", "--proxy", proxy.URL}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.NotZero(t, proxy.Requests())
}

func TestAccountNetworkCommandsRejectConflictingProxyFlags(t *testing.T) {
	tests := [][]string{
		{"pixiv", "auth", "add", "--proxy", "http://flag-proxy", "--no-proxy"},
		{"pixiv", "auth", "login", "--proxy", "http://flag-proxy", "--no-proxy"},
		{"pixiv", "auth", "check", "--proxy", "http://flag-proxy", "--no-proxy"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			useTempPaths(t)

			var stdout, stderr bytes.Buffer
			code := Run(args, strings.NewReader(""), &stdout, &stderr)

			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), "use either --proxy or --no-proxy, not both")
		})
	}
}

func TestAccountAddRejectsCookieInput(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add", "--token", "PHPSESSID=web; device_token=device"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), "cookie input is not supported; provide a Pixiv App API refresh token")
}

func TestRefreshTokenFlagsOnlyDescribeAppAPIInput(t *testing.T) {
	a := app{}
	commonCommand := &cobra.Command{}
	var commonOptions commandOptions
	a.bindCommonFlags(commonCommand, &commonOptions)
	common := commonCommand.Flags().Lookup("refresh-token")
	if common == nil {
		t.Fatal("missing --refresh-token flag")
	}
	assert.Equal(t, "Pixiv App API refresh token", common.Usage)

	add := a.newAccountAddCommand()
	token := add.Flags().Lookup("token")
	if token == nil {
		t.Fatal("missing auth add --token flag")
	}
	assert.Equal(t, "Pixiv App API refresh token", token.Usage)
}

func TestAccountPromptFlows(t *testing.T) {
	authPath, _ := useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"prompt-token": {userID: 444, username: "prompt-user"},
	})
	setPromptStub(t, promptStub{
		secrets:  []string{"prompt-token"},
		selects:  []string{"444 prompt-user", "444 prompt-user"},
		confirms: []bool{true},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(444), store.Accounts[0].UserID)

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
		DefaultUserID: 555,
		Accounts:      []auth.Account{{UserID: 555, Username: "kept-user", RefreshToken: "main-token"}},
	}))
	setPromptStub(t, promptStub{
		selects:  []string{"555 kept-user"},
		confirms: []bool{false},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "remove"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(555), store.Accounts[0].UserID)
}

func TestAccountLoginNoOpenStoresProfile(t *testing.T) {
	authPath, _ := useTempPaths(t)
	addr := freeLoopbackAddr(t)

	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/auth/token", r.URL.Path)
		require.NotEmpty(t, r.Header.Get("User-Agent"))
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
		assert.Equal(t, "callback-code", r.Form.Get("code"))
		assert.NotEmpty(t, r.Form.Get("code_verifier"))
		assert.Equal(t, "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback", r.Form.Get("redirect_uri"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-secret",
			"refresh_token": "refresh-secret",
			"user":          map[string]any{"id": "12345", "name": "oauth-user"},
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
	run := startAsyncCLIRun([]string{"pixiv", "auth", "login", "--addr", addr, "--no-open", "--timeout", "5s"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()

	page := waitForLoginServer(t, addr)
	loginURL, err := url.Parse(loginURLFromPage(t, page))
	require.NoError(t, err)
	callbackURL := "http://" + addr + "/callback?code=callback-code&state=" + url.QueryEscape(loginURL.Query().Get("state"))
	resp, err := http.Get(callbackURL)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.Contains(t, string(body), "已收到授权，正在回到 CLI 完成登录")
	assert.NotContains(t, string(body), "登录成功")
	assert.NotContains(t, string(body), "callback-code")

	require.Equal(t, 0, run.waitWithin(t, 5*time.Second), stderr.String())
	assert.False(t, calledOpen)
	assert.Contains(t, stderr.String(), "Browser opening is disabled")
	assert.Contains(t, stderr.String(), "Manual fallback page")
	assert.NotContains(t, stderr.String(), "Authorization code received; exchanging it for a refresh token.")
	assert.Equal(t, "登录成功（UID: 12345）\n", stdout.String())

	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Equal(t, int64(12345), store.DefaultUserID)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, "refresh-secret", store.Accounts[0].RefreshToken)
	assert.Equal(t, int64(12345), store.Accounts[0].UserID)
	assert.Equal(t, "oauth-user", store.Accounts[0].Username)
	assert.NotContains(t, stdout.String(), "refresh-secret")
	assert.NotContains(t, stderr.String(), "refresh-secret")
}

func TestAccountLoginProxyFlagOverridesEnvAndConfig(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n")))
	t.Setenv("https_proxy", "http://env-proxy")
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(loginOAuthSuccessHandler))
	defer oauth.Close()

	oldServices := newCLIServices
	var seenProxy string
	newCLIServices = func(logger *slog.Logger) application.Services {
		services := bootstrap.NewServices(logger)
		services.SDK.NewClient = func(request application.SDKClientRequest) (application.SDKClient, error) {
			if request.HTTPSProxyOverride != nil {
				seenProxy = *request.HTTPSProxyOverride
			}
			return publicpixiv.OpenDefault(publicpixiv.Options{HTTPClient: oauth.Client(), OAuthBaseURL: oauth.URL})
		}
		services.Login.SDK = services.SDK
		return services
	}
	t.Cleanup(func() { newCLIServices = oldServices })

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "auth", "login", "--addr", addr, "--no-open", "--timeout", "5s", "--proxy", "http://flag-proxy"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()

	waitForLoginServer(t, addr)
	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {"manual-code"}})
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	require.Equal(t, 0, run.waitWithin(t, 5*time.Second), stderr.String())
	assert.Equal(t, "http://flag-proxy", seenProxy)
}

func TestAccountLoginNoProxyFlagClearsEnvAndConfig(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, auth.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"http://file-proxy\"\n")))
	t.Setenv("https_proxy", "http://env-proxy")
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(loginOAuthSuccessHandler))
	defer oauth.Close()

	oldServices := newCLIServices
	seenProxy := "unset"
	newCLIServices = func(logger *slog.Logger) application.Services {
		services := bootstrap.NewServices(logger)
		services.SDK.NewClient = func(request application.SDKClientRequest) (application.SDKClient, error) {
			if request.HTTPSProxyOverride != nil {
				seenProxy = *request.HTTPSProxyOverride
			}
			return publicpixiv.OpenDefault(publicpixiv.Options{HTTPClient: oauth.Client(), OAuthBaseURL: oauth.URL})
		}
		services.Login.SDK = services.SDK
		return services
	}
	t.Cleanup(func() { newCLIServices = oldServices })

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "auth", "login", "--addr", addr, "--no-open", "--timeout", "5s", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()

	waitForLoginServer(t, addr)
	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {"manual-code"}})
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	require.Equal(t, 0, run.waitWithin(t, 5*time.Second), stderr.String())
	assert.Empty(t, seenProxy)
}

func TestAccountLoginBrowserFailureFallsBackToTerminalPrompt(t *testing.T) {
	authPath, _ := useTempPaths(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "terminal-code", r.Form.Get("code"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "prompt-refresh-secret",
			"user":          map[string]any{"id": "24680", "name": "terminal-user"},
		}))
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()
	restoreRelay := setTestURLSchemeRelayInstaller(t, func(context.Context, string) (func(), error) { return func() {}, nil })
	defer restoreRelay()
	restoreOpen := setTestOpenBrowser(t, func(string) error {
		return errors.New("opener unavailable")
	})
	defer restoreOpen()
	setPromptStub(t, promptStub{
		inputs: []string{"terminal-code"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "login", "--addr", addr, "--timeout", "5s"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(24680), store.Accounts[0].UserID)
	assert.Equal(t, "terminal-user", store.Accounts[0].Username)
	assert.Equal(t, "prompt-refresh-secret", store.Accounts[0].RefreshToken)
	assert.Contains(t, stderr.String(), "warning: could not open browser")
}

func TestAccountLoginBrowserSuccessStillAcceptsTerminalPrompt(t *testing.T) {
	authPath, _ := useTempPaths(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "pasted-code", r.Form.Get("code"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "pasted-refresh-secret",
			"user":          map[string]any{"id": "13579", "name": "pasted-user"},
		}))
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()
	restoreRelay := setTestURLSchemeRelayInstaller(t, func(context.Context, string) (func(), error) { return func() {}, nil })
	defer restoreRelay()

	opened := false
	openedURL := ""
	restoreOpen := setTestOpenBrowser(t, func(rawURL string) error {
		opened = true
		openedURL = rawURL
		require.Contains(t, rawURL, "code_challenge=")
		return nil
	})
	defer restoreOpen()
	setPromptStub(t, promptStub{
		inputs: []string{"pasted-code"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "login", "--addr", addr, "--timeout", "5s"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.True(t, opened)
	parsedLoginURL, err := url.Parse(openedURL)
	require.NoError(t, err)
	assert.Equal(t, publicpixiv.BuildLoginAuthorizationURL(parsedLoginURL.Query().Get("code_challenge"), parsedLoginURL.Query().Get("state")), openedURL)
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(13579), store.Accounts[0].UserID)
	assert.Equal(t, "pasted-user", store.Accounts[0].Username)
	assert.Equal(t, "pasted-refresh-secret", store.Accounts[0].RefreshToken)
	assert.NotContains(t, stdout.String(), "pasted-refresh-secret")
	assert.NotContains(t, stderr.String(), "pasted-refresh-secret")
}

func TestAccountLoginAcceptsPixivCallbackURLWithoutState(t *testing.T) {
	authPath, _ := useTempPaths(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "pixiv-callback-code", r.Form.Get("code"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "pixiv-callback-refresh-secret",
			"user":          map[string]any{"id": "97531", "name": "callback-user"},
		}))
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()
	restoreRelay := setTestURLSchemeRelayInstaller(t, func(context.Context, string) (func(), error) { return func() {}, nil })
	defer restoreRelay()

	restoreOpen := setTestOpenBrowser(t, func(string) error {
		return nil
	})
	defer restoreOpen()
	setPromptStub(t, promptStub{
		inputs: []string{"pixiv://account/login?code=pixiv-callback-code"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "login", "--addr", addr, "--timeout", "5s"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(97531), store.Accounts[0].UserID)
	assert.Equal(t, "callback-user", store.Accounts[0].Username)
	assert.Equal(t, "pixiv-callback-refresh-secret", store.Accounts[0].RefreshToken)
	assert.NotContains(t, stdout.String(), "pixiv-callback-refresh-secret")
	assert.NotContains(t, stderr.String(), "pixiv-callback-refresh-secret")
}

func TestAccountLoginManualPageRelaysPostRedirectThenAcceptsCode(t *testing.T) {
	authPath, _ := useTempPaths(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "manual-relay-code", r.Form.Get("code"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "manual-relay-refresh-secret",
			"user":          map[string]any{"id": "563412", "name": "manual-relay-user"},
		}))
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()
	restoreRelay := setTestURLSchemeRelayInstaller(t, func(context.Context, string) (func(), error) { return func() {}, nil })
	defer restoreRelay()

	// opener 由登录服务的 HTTP handler 调用，测试 goroutine 读取前须用同一把锁建立同步关系。
	var openedURLsMu sync.Mutex
	var openedURLs []string
	restoreOpen := setTestOpenBrowser(t, func(rawURL string) error {
		openedURLsMu.Lock()
		defer openedURLsMu.Unlock()
		openedURLs = append(openedURLs, rawURL)
		return nil
	})
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "auth", "login", "--addr", addr, "--timeout", "5s"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()

	page := waitForLoginServer(t, addr)
	loginURL := loginURLFromPage(t, page)
	returnTo := pixivAuthStartURLForTest(pixivLoginChallenge(loginURL))
	bridge := "https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape(returnTo)

	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {bridge}})
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Contains(t, string(body), "authorization relay opened")
	openedURLsMu.Lock()
	openedURLSnapshot := append([]string(nil), openedURLs...)
	openedURLsMu.Unlock()
	require.Contains(t, openedURLSnapshot, bridge)
	assert.Contains(t, stderr.String(), "Detected Pixiv authorization relay page")

	resp, err = http.PostForm("http://"+addr+"/manual", url.Values{"code": {"manual-relay-code"}})
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	require.Equal(t, 0, run.waitWithin(t, 5*time.Second), stderr.String())
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(563412), store.Accounts[0].UserID)
	assert.Equal(t, "manual-relay-user", store.Accounts[0].Username)
	assert.Equal(t, "manual-relay-refresh-secret", store.Accounts[0].RefreshToken)
	assert.NotContains(t, stdout.String(), "manual-relay-refresh-secret")
	assert.NotContains(t, stderr.String(), "manual-relay-refresh-secret")
}

func TestLoginCodeFromInputOnlyPixivCallbacksMayOmitState(t *testing.T) {
	accepts := func(rawURL string) bool {
		return rawURL == "pixiv://account/login?code=app-code" || rawURL == "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=https-code"
	}
	result := loginCodeFromInput("pixiv://account/login?code=app-code", accepts)
	require.NoError(t, result.err)
	assert.Equal(t, "pixiv://account/login?code=app-code", result.code)

	result = loginCodeFromInput("https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=https-code", accepts)
	require.NoError(t, result.err)
	assert.Equal(t, "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=https-code", result.code)

	result = loginCodeFromInput("http://127.0.0.1:12345/callback?code=loopback-code", accepts)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "does not match")

	result = loginCodeFromInput("pixiv://account/login?code=app-code&state=wrong-state", accepts)
	require.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "does not match")
}

func TestLoginInputFromTextRelaysPostRedirect(t *testing.T) {
	returnTo := pixivAuthStartURLForTest("expected-challenge")
	bridge := "https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape(returnTo)
	var opened []string

	result := loginInputFromText(bridge, acceptsTestCallback, "expected-challenge", func(rawURL string) error {
		opened = append(opened, rawURL)
		return nil
	})

	require.NoError(t, result.err)
	assert.True(t, result.relayed)
	assert.Empty(t, result.code)
	assert.Equal(t, []string{bridge}, opened)
}

func TestLoginInputFromTextRejectsInvalidPostRedirect(t *testing.T) {
	cases := []string{
		"https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape("https://example.test/web/v1/users/auth/pixiv/start"),
		"https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape("https://app-api.pixiv.net/not-start"),
		"https://accounts.pixiv.net/post-redirect",
	}
	for _, input := range cases {
		var opened []string
		result := loginInputFromText(input, acceptsTestCallback, "expected-challenge", func(rawURL string) error {
			opened = append(opened, rawURL)
			return nil
		})

		require.Error(t, result.err)
		assert.True(t, result.relayed)
		assert.Empty(t, opened)
	}
}

func TestLoginInputFromTextRejectsStalePostRedirectChallenge(t *testing.T) {
	returnTo := pixivAuthStartURLForTest("stale-challenge")
	bridge := "https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape(returnTo)
	var opened []string

	result := loginInputFromText(bridge, acceptsTestCallback, "expected-challenge", func(rawURL string) error {
		opened = append(opened, rawURL)
		return nil
	})

	require.Error(t, result.err)
	assert.True(t, result.relayed)
	assert.Empty(t, opened)
}

func TestPixivPostRedirectReturnToAcceptsOnlyPixivStartURL(t *testing.T) {
	returnTo := "https://app-api.pixiv.net/web/v1/users/auth/pixiv/start?code_challenge=challenge&client=pixiv-android"
	actual, ok := pixivPostRedirectReturnTo("https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape(returnTo))
	require.True(t, ok)
	assert.Equal(t, returnTo, actual)

	_, ok = pixivPostRedirectReturnTo("https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape("https://example.test/web/v1/users/auth/pixiv/start"))
	assert.False(t, ok)

	_, ok = pixivPostRedirectReturnTo("https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape("https://app-api.pixiv.net/not-start"))
	assert.False(t, ok)
}

func TestPixivAuthStartMatchesChallenge(t *testing.T) {
	assert.True(t, pixivAuthStartMatchesChallenge(pixivAuthStartURLForTest("current-challenge"), "current-challenge"))
	assert.False(t, pixivAuthStartMatchesChallenge(pixivAuthStartURLForTest("stale-challenge"), "current-challenge"))
	assert.True(t, pixivAuthStartMatchesChallenge(pixivAuthStartURLForTest("any-challenge"), ""))
}

func assertPrivateFileMode(t *testing.T, actual os.FileMode, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Windows mode bits 不作为 ACL 证据；DACL 边界由平台持久化契约说明。
		return
	}
	assert.Equal(t, want, actual)
}

func pixivAuthStartURLForTest(challenge string) string {
	values := url.Values{}
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("client", "pixiv-android")
	values.Set("via", "login")
	return "https://app-api.pixiv.net/web/v1/users/auth/pixiv/start?" + values.Encode()
}

func setTestOAuthBase(t *testing.T, baseURL string) func() {
	t.Helper()
	old := newCLIServices
	newCLIServices = func(logger *slog.Logger) application.Services {
		services := old(logger)
		services.SDK.NewClient = func(request application.SDKClientRequest) (application.SDKClient, error) {
			return publicpixiv.OpenDefault(publicpixiv.Options{
				UserID:       request.UserID,
				RefreshToken: request.RefreshToken,
				AuthFilePath: request.AuthFilePath,
				OAuthBaseURL: baseURL,
				Logger:       logger,
			})
		}
		services.Login.SDK = services.SDK
		return services
	}
	return func() { newCLIServices = old }
}

func setTestOpenBrowser(t *testing.T, opener func(string) error) func() {
	t.Helper()
	return setOpenBrowserForTest(opener)
}

func setTestURLSchemeRelayInstaller(t *testing.T, installer urlSchemeRelayInstaller) func() {
	t.Helper()
	return setURLSchemeRelayInstallerForTest(installer)
}

// setTestPublicSDKFactory 保持 CLI 测试走与生产相同的 public OpenDefault 路径。
// 测试仅替换上游地址；传入的 proxy 覆写仍由生产 HTTPClient 构造并经真实 transport
// 发出请求，避免以接口 fake 掩盖 --proxy/--no-proxy 的装配错误。
func setTestPublicSDKFactory(t *testing.T, oauthBaseURL, appAPIBaseURL, webAPIBaseURL string, resourcePolicy publicpixiv.ResourcePolicy, observe func(application.SDKClientRequest)) {
	setTestPublicSDKFactoryWithHTTPClient(t, oauthBaseURL, appAPIBaseURL, webAPIBaseURL, resourcePolicy, internalpixiv.HTTPClient, observe)
}

func setTestPublicSDKFactoryWithHTTPClient(t *testing.T, oauthBaseURL, appAPIBaseURL, webAPIBaseURL string, resourcePolicy publicpixiv.ResourcePolicy, newHTTPClient func(string) (*http.Client, error), observe func(application.SDKClientRequest)) {
	t.Helper()
	authPath, err := auth.AuthFilePath()
	require.NoError(t, err)
	configPath, err := config.ConfigFilePath()
	require.NoError(t, err)
	old := newCLIServices
	newCLIServices = func(logger *slog.Logger) application.Services {
		services := bootstrap.NewServices(logger)
		services.SDK.NewClient = func(request application.SDKClientRequest) (application.SDKClient, error) {
			if observe != nil {
				observe(request)
			}
			options := publicpixiv.Options{
				UserID:         request.UserID,
				RefreshToken:   request.RefreshToken,
				AuthFilePath:   authPath,
				ConfigFilePath: configPath,
				OAuthBaseURL:   oauthBaseURL,
				AppAPIBaseURL:  appAPIBaseURL,
				WebAPIBaseURL:  webAPIBaseURL,
				ResourcePolicy: resourcePolicy,
				Logger:         logger,
			}
			if request.HTTPSProxyOverride != nil {
				httpClient, err := newHTTPClient(*request.HTTPSProxyOverride)
				if err != nil {
					return nil, err
				}
				options.HTTPClient = httpClient
			}
			return publicpixiv.OpenDefault(options)
		}
		// Account/Login 各自持有 SDKService 值；重新装配后必须同步替换它们。
		services.Account.SDK = services.SDK
		services.Login.SDK = services.SDK
		return services
	}
	t.Cleanup(func() { newCLIServices = old })
}

type testForwardProxy struct {
	*httptest.Server
	mu       sync.Mutex
	requests int
}

// newTestForwardProxy 是最小 HTTP forward proxy：它让测试验证 SDK transport
// 确实经过 --proxy 指定的地址，而不是仅观察 factory 入参。
func newTestForwardProxy(t *testing.T) *testForwardProxy {
	t.Helper()
	proxy := &testForwardProxy{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	proxy.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.mu.Lock()
		proxy.requests++
		proxy.mu.Unlock()
		if r.Method == http.MethodConnect {
			proxy.tunnel(t, w, r)
			return
		}

		outbound := r.Clone(r.Context())
		outbound.RequestURI = ""
		outbound.Host = ""
		response, err := transport.RoundTrip(outbound)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		for key, values := range response.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}))
	t.Cleanup(proxy.Close)
	return proxy
}

func (p *testForwardProxy) tunnel(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	upstream, err := net.Dial("tcp", r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Close()
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "response writer does not support connection hijacking", http.StatusInternalServerError)
		return
	}
	client, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
	go func() {
		_, _ = io.Copy(upstream, client)
		_ = upstream.Close()
	}()
	_, _ = io.Copy(client, upstream)
}

func (p *testForwardProxy) Requests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests
}

type authIdentity struct {
	userID   int64
	username string
}

func setTestAuthClientFactory(t *testing.T, identities map[string]authIdentity) {
	t.Helper()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			require.NoError(t, r.ParseForm())
			token := r.Form.Get("refresh_token")
			identity, ok := identities[token]
			require.True(t, ok, "unexpected refresh token %q", token)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-" + token,
				"refresh_token": token,
				"user":          map[string]any{"id": identity.userID, "name": identity.username},
			}))
		case "/v1/user/detail":
			uid := r.URL.Query().Get("user_id")
			for _, identity := range identities {
				if uid == fmt.Sprint(identity.userID) {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"user":              map[string]any{"id": identity.userID, "name": identity.username},
						"profile":           map[string]any{},
						"profile_publicity": map[string]any{},
						"workspace":         map[string]any{},
					}))
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)
	setTestPublicSDKFactory(t, api.URL, api.URL, api.URL, publicpixiv.ResourcePolicy{}, nil)
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

// acceptsTestCallback 只模拟 browser adapter 已从 LoginSession 得到的布尔校验，
// 不在 CLI helper 测试里重新读取或比较 state。
func acceptsTestCallback(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && isBrowserCallbackURL(parsed) && strings.TrimSpace(parsed.Query().Get("code")) != ""
}
