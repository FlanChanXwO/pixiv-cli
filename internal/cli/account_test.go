package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
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
	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
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

func TestAccountImportAcceptsPositionalRefreshToken(t *testing.T) {
	authPath, _ := useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"opaque/import-token": {userID: 333, username: "import-user"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", "opaque/import-token"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "added uid:333\nusername:import-user\n", stdout.String())
	assert.Empty(t, stderr.String())
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(333), store.Accounts[0].UserID)
	assert.Equal(t, "import-user", store.Accounts[0].Username)
	assert.Equal(t, "opaque/import-token", store.Accounts[0].RefreshToken)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "import", "opaque/import-token"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "updated uid:333\nusername:import-user\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestAccountImportJSONDoesNotEchoInputOrRotatedToken(t *testing.T) {
	useTempPaths(t)
	t.Setenv("PIXIV_LOG_LEVEL", "info")
	const inputToken = "input-token-canary"
	const rotatedToken = "rotated-token-canary"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/auth/token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, inputToken, r.Form.Get("refresh_token"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-token-canary",
			"refresh_token": rotatedToken,
			"user":          map[string]any{"id": 335, "name": "redacted-user"},
		}))
	}))
	defer api.Close()
	setTestPublicSDKFactory(t, api.URL, api.URL, api.URL, publicpixiv.ResourcePolicy{}, nil)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", inputToken, "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.JSONEq(t, `{"user_id":335,"username":"redacted-user","status":"added"}`, stdout.String())
	for _, canary := range []string{inputToken, rotatedToken, "access-token-canary"} {
		assert.NotContains(t, stdout.String(), canary)
		assert.NotContains(t, stderr.String(), canary)
	}
}

func TestAccountImportStdinRemovesOnlyOneTrailingLF(t *testing.T) {
	for name, input := range map[string]string{
		"LF":   "  opaque stdin token  \n",
		"CRLF": "  opaque stdin token  \r\n",
	} {
		t.Run(name, func(t *testing.T) {
			token, err := readRefreshTokenInput(strings.NewReader(input))
			require.NoError(t, err)
			assert.Equal(t, "  opaque stdin token  ", token)
		})
	}
}

func TestAccountImportStdinRejectsAdditionalNewline(t *testing.T) {
	_, err := readRefreshTokenInput(strings.NewReader("first-line\nsecond-line\n"))
	require.EqualError(t, err, "refresh token input must contain exactly one line")
}

func TestAccountImportStdinRejectsEmptyInput(t *testing.T) {
	_, err := readRefreshTokenInput(strings.NewReader(""))
	require.EqualError(t, err, "refresh token cannot be empty")
}

func TestAccountAddIsRemovedWithoutEchoingToken(t *testing.T) {
	useTempPaths(t)
	const token = "removed-command-token-canary"

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "add", token}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.NotContains(t, stdout.String(), token)
	assert.NotContains(t, stderr.String(), token)
}

func TestAuthTokenCommandIsRemoved(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "token"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.NotContains(t, stderr.String(), "refresh_token")
}

func TestAccountImportRejectsLegacyFlagAndExtraArgumentWithoutEchoingTokens(t *testing.T) {
	for name, args := range map[string][]string{
		"legacy token flag": {"pixiv", "auth", "import", "--token", "legacy-flag-token-canary"},
		"extra argument":    {"pixiv", "auth", "import", "first-token-canary", "second-token-canary"},
	} {
		t.Run(name, func(t *testing.T) {
			useTempPaths(t)
			var stdout, stderr bytes.Buffer
			code := Run(args, strings.NewReader(""), &stdout, &stderr)

			require.NotZero(t, code)
			for _, canary := range []string{"legacy-flag-token-canary", "first-token-canary", "second-token-canary"} {
				assert.NotContains(t, stdout.String(), canary)
				assert.NotContains(t, stderr.String(), canary)
			}
		})
	}
}

func TestAccountImportListUseRemovePreservesOrder(t *testing.T) {
	authPath, _ := useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"main/token":  {userID: 111, username: "alice"},
		"other-token": {userID: 222, username: "bob"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", "main/token"}, strings.NewReader(""), &stdout, &stderr)
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
	code = Run([]string{"pixiv", "auth", "import", "other-token"}, strings.NewReader(""), &stdout, &stderr)
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

func TestAccountImportReadsTokenFromStdin(t *testing.T) {
	authPath, _ := useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"stdin-token": {userID: 333, username: "stdin-user"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import"}, strings.NewReader("stdin-token\n"), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	assert.Equal(t, int64(333), store.Accounts[0].UserID)
	assert.Equal(t, "stdin-user", store.Accounts[0].Username)
	assert.Equal(t, "stdin-token", store.Accounts[0].RefreshToken)
}

func TestAccountImportProxyFlagOverridesEnvAndConfig(t *testing.T) {
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
	code := Run([]string{"pixiv", "auth", "import", "input-token", "--proxy", proxy.URL}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.NotZero(t, proxy.Requests())
}

func TestAccountImportNoProxyFlagClearsEnvAndConfig(t *testing.T) {
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
	code := Run([]string{"pixiv", "auth", "import", "input-token", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr)

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
		{"pixiv", "auth", "import", "--proxy", "http://flag-proxy", "--no-proxy"},
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

func TestAccountImportRejectsCookieInput(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", "PHPSESSID=web; device_token=device"}, strings.NewReader(""), &stdout, &stderr)

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

	importCommand := a.newAccountImportCommand()
	assert.Nil(t, importCommand.Flags().Lookup("token"))
}

func TestAuthExportPrintsOnlyDefaultStoredRefreshToken(t *testing.T) {
	authPath, _ := useTempPaths(t)
	t.Setenv("PIXIV_LOG_LEVEL", "info")
	t.Setenv("PIXIV_REFRESH_TOKEN", "environment-token-must-be-ignored")
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 444,
		Accounts:      []auth.Account{{UserID: 444, Username: "stored", RefreshToken: "opaque/default-token"}},
	}))
	before, err := os.ReadFile(authPath)
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "opaque/default-token\n", stdout.String())
	assert.Empty(t, stderr.String())
	after, err := os.ReadFile(authPath)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestAuthExportRawTokenPropagatesStdoutWriterErrorSafely(t *testing.T) {
	authPath, _ := useTempPaths(t)
	const token = "raw-writer-error-secret"
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 444,
		Accounts:      []auth.Account{{UserID: 444, RefreshToken: token}},
	}))
	stdout := &syntheticFailingWriter{err: errors.New(token)}
	directErr := writeAuthExportStdout(stdout, []byte(token))
	require.ErrorIs(t, directErr, stdout.err)
	assert.NotContains(t, directErr.Error(), token)
	var stderr bytes.Buffer

	code := Run([]string{"pixiv", "auth", "export"}, strings.NewReader(""), stdout, &stderr)

	require.Equal(t, 1, code)
	assert.NotContains(t, stderr.String(), token)
	assert.Contains(t, stderr.String(), "write stdout")
}

type syntheticFailingWriter struct{ err error }

func (w *syntheticFailingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestAuthExportBundleAndSummaryPropagateShortStdoutWritesSafely(t *testing.T) {
	authPath, _ := useTempPaths(t)
	const token = "short-writer-secret"
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 444,
		Accounts:      []auth.Account{{UserID: 444, RefreshToken: token}},
	}))

	for name, args := range map[string][]string{
		"bundle":  {"pixiv", "auth", "export", "--all"},
		"summary": {"pixiv", "auth", "export", "--output", filepath.Join(t.TempDir(), "export.json")},
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := Run(args, strings.NewReader(""), syntheticShortWriter{}, &stderr)

			require.Equal(t, 1, code)
			assert.Contains(t, stderr.String(), "write stdout failed")
			assert.NotContains(t, stderr.String(), token)
		})
	}
	directErr := writeAuthExportStdout(syntheticShortWriter{}, []byte(token))
	require.ErrorIs(t, directErr, io.ErrShortWrite)
	assert.NotContains(t, directErr.Error(), token)
}

type syntheticShortWriter struct{}

func (syntheticShortWriter) Write(body []byte) (int, error) { return len(body) - 1, nil }

func TestAuthExportAllBundleCanBeRestoredFromFile(t *testing.T) {
	authPath, _ := useTempPaths(t)
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 444,
		Accounts: []auth.Account{
			{UserID: 444, Username: "first", RefreshToken: "first-export-secret"},
			{UserID: 555, Username: "second", RefreshToken: "second-export-secret"},
		},
	}))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "export", "--all"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Empty(t, stderr.String())
	assert.True(t, strings.HasSuffix(stdout.String(), "\n"))
	bundlePath := filepath.Join(t.TempDir(), "auth-export.json")
	require.NoError(t, os.WriteFile(bundlePath, stdout.Bytes(), 0o600))
	require.NoError(t, os.Remove(authPath))

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "import", "--file", bundlePath}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "added uid:444\nadded uid:555\ndefault uid: 444\n", stdout.String())
	assert.Empty(t, stderr.String())

	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	assert.Equal(t, int64(444), store.DefaultUserID)
	require.Len(t, store.Accounts, 2)
	assert.Equal(t, "first-export-secret", store.Accounts[0].RefreshToken)
	assert.Equal(t, "second-export-secret", store.Accounts[1].RefreshToken)
}

func TestAuthImportBundleFromStdinPrintsSafeJSONReport(t *testing.T) {
	authPath, _ := useTempPaths(t)
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 888,
		Accounts:      []auth.Account{{UserID: 888, Username: "before", RefreshToken: "old-local-secret"}},
	}))
	const bundle = `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":777,"accounts":[{"user_id":777,"username":"new","refresh_token":"stdin-bundle-secret"},{"user_id":888,"username":"updated","refresh_token":"updated-bundle-secret"}]}`

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", "--file", "-", "--json"}, strings.NewReader(bundle), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.JSONEq(t, `{"accounts":[{"user_id":777,"username":"new","status":"added"},{"user_id":888,"username":"updated","status":"updated"}],"default_user_id":888}`, stdout.String())
	assert.NotContains(t, stdout.String(), "stdin-bundle-secret")
	assert.NotContains(t, stderr.String(), "stdin-bundle-secret")
	assert.NotContains(t, stdout.String(), "updated-bundle-secret")
	assert.NotContains(t, stderr.String(), "updated-bundle-secret")

	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 2)
	_, added, ok := store.Get(777)
	require.True(t, ok)
	assert.Equal(t, "stdin-bundle-secret", added.RefreshToken)
	_, updated, ok := store.Get(888)
	require.True(t, ok)
	assert.Equal(t, "updated-bundle-secret", updated.RefreshToken)
}

func TestAuthImportBundleRejectsTokenAndProxyFlagsWithoutReadingSecrets(t *testing.T) {
	for name, args := range map[string][]string{
		"positional token": {"pixiv", "auth", "import", "positional-secret", "--file", "private-path-secret"},
		"proxy":            {"pixiv", "auth", "import", "--file", "private-path-secret", "--proxy", "http://user:proxy-secret@example.invalid"},
		"no proxy":         {"pixiv", "auth", "import", "--file", "private-path-secret", "--no-proxy"},
	} {
		t.Run(name, func(t *testing.T) {
			useTempPaths(t)
			var stdout, stderr bytes.Buffer
			code := Run(args, strings.NewReader("stdin-secret"), &stdout, &stderr)

			require.Equal(t, 1, code)
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), "--file cannot be combined")
			for _, secret := range []string{"positional-secret", "private-path-secret", "proxy-secret", "stdin-secret"} {
				assert.NotContains(t, stderr.String(), secret)
			}
		})
	}
}

func TestAuthImportBundleMissingFileReportsSafeStableCategory(t *testing.T) {
	useTempPaths(t)
	privatePath := filepath.Join(t.TempDir(), "private-missing-bundle-secret.json")
	stdin := &syntheticMustNotRead{err: errors.New("stdin-must-not-be-read")}
	var stdout, stderr bytes.Buffer

	code := Run([]string{"pixiv", "auth", "import", "--file", privatePath}, stdin, &stdout, &stderr)

	require.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "error: read authentication export bundle failed: not_found\n")
	assert.NotContains(t, stderr.String(), privatePath)
	assert.NotContains(t, stderr.String(), "stdin-must-not-be-read")
	assert.Zero(t, stdin.calls)
}

type syntheticMustNotRead struct {
	calls int
	err   error
}

func (r *syntheticMustNotRead) Read([]byte) (int, error) {
	r.calls++
	return 0, r.err
}

func TestAuthImportBundleDirectoryReportsSafeStableCategory(t *testing.T) {
	useTempPaths(t)
	privatePath := filepath.Join(t.TempDir(), "private-bundle-directory-secret")
	require.NoError(t, os.Mkdir(privatePath, 0o700))
	var stdout, stderr bytes.Buffer

	code := Run([]string{"pixiv", "auth", "import", "--file", privatePath}, strings.NewReader("stdin-must-not-be-read"), &stdout, &stderr)

	require.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "error: read authentication export bundle failed: is_directory\n")
	assert.NotContains(t, stderr.String(), privatePath)
	assert.NotContains(t, stderr.String(), "stdin-must-not-be-read")
}

func TestAuthBundleFileReadErrorsExposeTypedSafeCategories(t *testing.T) {
	privatePath := "/private/path/bundle-secret.json"
	for _, test := range []struct {
		name     string
		cause    error
		category string
	}{
		{name: "permission", cause: &fs.PathError{Op: "open", Path: privatePath, Err: fs.ErrPermission}, category: "permission_denied"},
		{name: "other io", cause: errors.New("other-io-secret-cause"), category: "io"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := classifyAuthBundleFileReadError(privatePath, test.cause)
			var typed authBundleFileReadError
			require.ErrorAs(t, err, &typed)
			assert.Equal(t, test.category, typed.Category())
			assert.ErrorIs(t, err, test.cause)
			assert.NotContains(t, err.Error(), privatePath)
			assert.NotContains(t, err.Error(), "other-io-secret-cause")
		})
	}
}

func TestAuthExportOutputRequiresForceAndPrintsOnlySafeSummary(t *testing.T) {
	authPath, _ := useTempPaths(t)
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 888,
		Accounts:      []auth.Account{{UserID: 888, Username: "exported", RefreshToken: "output-secret-canary"}},
	}))
	directory := filepath.Join(t.TempDir(), "destination")
	require.NoError(t, os.Mkdir(directory, 0o751))
	outputPath := filepath.Join(directory, "auth-export.json")
	require.NoError(t, os.WriteFile(outputPath, []byte("old-body"), 0o644))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "export", "--output", outputPath}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.NotContains(t, stderr.String(), outputPath)
	assert.NotContains(t, stderr.String(), "output-secret-canary")
	body, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "old-body", string(body))

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "export", "--output", outputPath, "--force"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "output: "+outputPath+"\naccounts: 1\n", stdout.String())
	assert.Empty(t, stderr.String())
	exportedBody, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	bundle, err := publicpixiv.DecodeAuthExportBundle(exportedBody)
	require.NoError(t, err)
	require.Len(t, bundle.Accounts, 1)
	assert.Equal(t, "output-secret-canary", bundle.Accounts[0].RefreshToken)
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(outputPath)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		parent, statErr := os.Stat(directory)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o751), parent.Mode().Perm())
	}
}

func TestAuthExportIgnoresInvalidLoggingConfiguration(t *testing.T) {
	authPath, configPath := useTempPaths(t)
	t.Setenv("PIXIV_LOG_LEVEL", "loud")
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 444,
		Accounts:      []auth.Account{{UserID: 444, RefreshToken: "synthetic-invalid-refresh-token"}},
	}))
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[logging]\nlevel = 'loud'\n")))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "synthetic-invalid-refresh-token\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestAuthExportSelectsExplicitUIDAndRejectsNonContractInput(t *testing.T) {
	authPath, _ := useTempPaths(t)
	t.Setenv("PIXIV_LOG_LEVEL", "info")
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 444,
		Accounts: []auth.Account{
			{UserID: 444, RefreshToken: "default-secret"},
			{UserID: 555, RefreshToken: "selected-secret"},
		},
	}))

	t.Run("explicit uid", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"pixiv", "auth", "export", "555"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		assert.Equal(t, "selected-secret\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	for name, args := range map[string][]string{
		"invalid uid":   {"pixiv", "auth", "export", "not-a-uid"},
		"too many args": {"pixiv", "auth", "export", "444", "555"},
		"json flag":     {"pixiv", "auth", "export", "--json"},
		"proxy flag":    {"pixiv", "auth", "export", "--proxy", "http://localhost:7890"},
		"no proxy flag": {"pixiv", "auth", "export", "--no-proxy"},
		"all with uid":  {"pixiv", "auth", "export", "444", "--all"},
		"force stdout":  {"pixiv", "auth", "export", "--force"},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(args, strings.NewReader(""), &stdout, &stderr)
			assert.Equal(t, 1, code)
			assert.Empty(t, stdout.String())
			assert.NotEmpty(t, stderr.String())
			assert.NotContains(t, stderr.String(), "default-secret")
			assert.NotContains(t, stderr.String(), "selected-secret")
		})
	}
}

func TestAuthExportInvalidUIDDoesNotEchoSensitiveInput(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[logging]\nlevel = 'warn'\n")))

	for name, canary := range map[string]string{
		"token-like": "synthetic-refresh-token-secret",
		"path-like":  "/synthetic/private/auth-secret.json",
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run([]string{"pixiv", "auth", "export", canary}, strings.NewReader(""), &stdout, &stderr)

			assert.Equal(t, 1, code)
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), canary)
			assert.Equal(t, "error: uid must be a positive integer\n", stderr.String())
		})
	}
}

func TestAuthExportHelpAndArgumentErrorsIgnoreInvalidLoggingConfiguration(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[logging]\nlevel = 'loud'\n")))

	t.Run("help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"pixiv", "auth", "export", "--help"}, strings.NewReader(""), &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		assert.Contains(t, stdout.String(), "pixiv auth export [UID]")
		assert.Empty(t, stderr.String())
	})

	t.Run("argument error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"pixiv", "auth", "export", "1", "2"}, strings.NewReader(""), &stdout, &stderr)
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout.String())
		assert.Equal(t, "error: usage: pixiv auth export [UID]\n", stderr.String())
		assert.NotContains(t, stderr.String(), "log_level")
	})
}

func TestAuthExportLocalStateErrorsAreSafe(t *testing.T) {
	const storedTokenCanary = "synthetic-stored-token-secret"
	const malformedCanary = "synthetic-malformed-auth-secret"

	for _, test := range []struct {
		name      string
		args      []string
		prepare   func(*testing.T, string)
		wantError string
		forbidden string
	}{
		{
			name:      "default account missing",
			args:      []string{"pixiv", "auth", "export"},
			prepare:   func(*testing.T, string) {},
			wantError: "pixiv unauthorized operation=export_auth_bundle",
		},
		{
			name: "explicit account missing",
			args: []string{"pixiv", "auth", "export", "999"},
			prepare: func(t *testing.T, authPath string) {
				require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
					DefaultUserID: 444,
					Accounts:      []auth.Account{{UserID: 444, RefreshToken: storedTokenCanary}},
				}))
			},
			wantError: "pixiv invalid_argument operation=export_auth_bundle user_id=999",
			forbidden: storedTokenCanary,
		},
		{
			name: "malformed auth store",
			args: []string{"pixiv", "auth", "export"},
			prepare: func(t *testing.T, authPath string) {
				require.NoError(t, auth.WritePrivateFile(authPath, []byte(`{"accounts":[`+malformedCanary+`]}`)))
			},
			wantError: "local_state_kind=auth_malformed",
			forbidden: malformedCanary,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			authPath, _ := useTempPaths(t)
			test.prepare(t, authPath)
			var stdout, stderr bytes.Buffer
			code := Run(test.args, strings.NewReader(""), &stdout, &stderr)

			assert.Equal(t, 1, code)
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), test.wantError)
			if test.forbidden != "" {
				assert.NotContains(t, stderr.String(), test.forbidden)
			}
			assert.NotContains(t, stderr.String(), authPath)
		})
	}
}

func TestAuthExportPermissionDeniedIsTypedAndSafe(t *testing.T) {
	const tokenCanary = "synthetic-permission-token-secret"
	authPath, _ := useTempPaths(t)
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 444,
		Accounts:      []auth.Account{{UserID: 444, RefreshToken: tokenCanary}},
	}))
	restore := auth.SetReadAuthStoreFileForTest(authPath, func(path string) ([]byte, error) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}
	})
	t.Cleanup(restore)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "error: pixiv invalid_argument operation=export_auth_bundle local_state_kind=permission_denied\n", stderr.String())
	assert.NotContains(t, stderr.String(), authPath)
	assert.NotContains(t, stderr.String(), tokenCanary)
}

func TestAuthExportRejectsBlankSelectedTokenSafely(t *testing.T) {
	for name, token := range map[string]string{"empty": "", "whitespace": "   "} {
		t.Run(name, func(t *testing.T) {
			authPath, _ := useTempPaths(t)
			body := fmt.Sprintf(`{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":%q}]}`, token)
			require.NoError(t, auth.WritePrivateFile(authPath, []byte(body)))

			var stdout, stderr bytes.Buffer
			code := Run([]string{"pixiv", "auth", "export", "7"}, strings.NewReader(""), &stdout, &stderr)

			assert.Equal(t, 1, code)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "error: pixiv invalid_argument operation=export_auth_bundle local_state_kind=auth_malformed\n", stderr.String())
			assert.NotContains(t, stderr.String(), authPath)
			if token != "" {
				assert.NotContains(t, stderr.String(), token)
			}
		})
	}
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
	code := Run([]string{"pixiv", "auth", "import"}, strings.NewReader(""), &stdout, &stderr)
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
	// 最终页在 OAuth 真正完成后返回；成功标题居中，且不回显 code。
	assert.Contains(t, string(body), "Login successful")
	assert.Contains(t, string(body), "text-align:center")
	assert.NotContains(t, string(body), "callback-code")
	assert.NotContains(t, string(body), "refresh-secret")

	require.Equal(t, 0, run.waitWithin(t, 5*time.Second), stderr.String())
	assert.False(t, calledOpen)
	assert.Contains(t, stderr.String(), "Browser opening is disabled")
	assert.Contains(t, stderr.String(), "Manual fallback page")
	assert.NotContains(t, stderr.String(), "Authorization code received; exchanging it for a refresh token.")
	assert.Equal(t, "\nLogin successful (UID: 12345)\n", stdout.String())

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

func TestAccountLoginRelayInstallerFailureKeepsBrowserAndManualFallback(t *testing.T) {
	authPath, _ := useTempPaths(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "manual-fallback-code", r.Form.Get("code"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "manual-fallback-refresh-secret",
			"user":          map[string]any{"id": "86420", "name": "manual-fallback-user"},
		}))
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()

	installedEndpoint := make(chan string, 1)
	restoreRelay := setTestURLSchemeRelayInstaller(t, func(_ context.Context, manualURL string) (func(), error) {
		installedEndpoint <- manualURL
		return nil, errors.New("test helper install unavailable")
	})
	defer restoreRelay()

	openedURL := make(chan string, 1)
	restoreOpen := setTestOpenBrowser(t, func(rawURL string) error {
		openedURL <- rawURL
		return nil
	})
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "auth", "login", "--addr", addr, "--timeout", "5s"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()

	page := waitForLoginServer(t, addr)
	loginURL := loginURLFromPage(t, page)

	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {"manual-fallback-code"}})
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	require.Equal(t, 0, run.waitWithin(t, 5*time.Second), stderr.String())
	require.Len(t, installedEndpoint, 1)
	assert.Equal(t, "http://"+addr+"/callback", <-installedEndpoint)
	require.Len(t, openedURL, 1)
	assert.Equal(t, loginURL, <-openedURL)
	assert.Contains(t, stderr.String(), "warning: pixiv:// callback handler is unavailable: test helper install unavailable")
	assert.Contains(t, stderr.String(), "Manual fallback page: http://"+addr+"/")
	assert.Equal(t, "\nLogin successful (UID: 86420)\n", stdout.String())

	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(86420), store.Accounts[0].UserID)
	assert.Equal(t, "manual-fallback-user", store.Accounts[0].Username)
	assert.Equal(t, "manual-fallback-refresh-secret", store.Accounts[0].RefreshToken)
	assert.NotContains(t, stdout.String(), "manual-fallback-refresh-secret")
	assert.NotContains(t, stderr.String(), "manual-fallback-refresh-secret")
}

func TestAccountLoginHelperRelayShowsFinalSuccessPageInBrowser(t *testing.T) {
	useTempPaths(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "helper-relay-code", r.Form.Get("code"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "helper-relay-refresh-secret",
			"user":          map[string]any{"id": "97542", "name": "helper-relay-user"},
		}))
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()

	helperEndpoint := make(chan string, 1)
	restoreRelay := setTestURLSchemeRelayInstaller(t, func(_ context.Context, endpoint string) (func(), error) {
		helperEndpoint <- endpoint
		return func() {}, nil
	})
	defer restoreRelay()
	restoreOpen := setTestOpenBrowser(t, func(string) error { return nil })
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "auth", "login", "--addr", addr, "--timeout", "5s"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()

	waitForLoginServer(t, addr)
	endpoint := <-helperEndpoint
	require.Equal(t, "http://"+addr+"/callback", endpoint)

	bridgeResp, err := http.Get(endpoint)
	require.NoError(t, err)
	bridge, _ := io.ReadAll(bridgeResp.Body)
	_ = bridgeResp.Body.Close()
	require.Equal(t, http.StatusOK, bridgeResp.StatusCode, string(bridge))
	assert.Contains(t, string(bridge), "Completing login")
	assert.Contains(t, string(bridge), "window.location.hash")
	assert.Contains(t, string(bridge), "form.action = \"/manual\"")

	finalResp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {"pixiv://account/login?code=helper-relay-code"}})
	require.NoError(t, err)
	finalPage, _ := io.ReadAll(finalResp.Body)
	_ = finalResp.Body.Close()
	require.Equal(t, http.StatusOK, finalResp.StatusCode, string(finalPage))
	assert.Contains(t, string(finalPage), "Login successful")
	require.Equal(t, 0, run.waitWithin(t, 5*time.Second), stderr.String())
	assert.Equal(t, "\nLogin successful (UID: 97542)\n", stdout.String())
	assert.NotContains(t, stdout.String(), "helper-relay-refresh-secret")
	assert.NotContains(t, stderr.String(), "helper-relay-refresh-secret")
}

func TestAccountLoginHelperRelayShowsFinalFailurePageInBrowser(t *testing.T) {
	useTempPaths(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "helper-relay-failure-code", r.Form.Get("code"))
		http.Error(w, "upstream failure canary", http.StatusBadGateway)
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()

	helperEndpoint := make(chan string, 1)
	restoreRelay := setTestURLSchemeRelayInstaller(t, func(_ context.Context, endpoint string) (func(), error) {
		helperEndpoint <- endpoint
		return func() {}, nil
	})
	defer restoreRelay()
	restoreOpen := setTestOpenBrowser(t, func(string) error { return nil })
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "auth", "login", "--addr", addr, "--timeout", "5s"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()

	waitForLoginServer(t, addr)
	require.Equal(t, "http://"+addr+"/callback", <-helperEndpoint)
	finalResp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {"pixiv://account/login?code=helper-relay-failure-code"}})
	require.NoError(t, err)
	finalPage, _ := io.ReadAll(finalResp.Body)
	_ = finalResp.Body.Close()
	require.Equal(t, http.StatusBadRequest, finalResp.StatusCode, string(finalPage))
	assert.Contains(t, string(finalPage), "Login failed")
	assert.NotContains(t, string(finalPage), "upstream failure canary")
	assert.NotContains(t, string(finalPage), "helper-relay-failure-code")
	require.NotZero(t, run.waitWithin(t, 5*time.Second), stderr.String())
	assert.Empty(t, stdout.String())
	assert.NotContains(t, stderr.String(), "helper-relay-failure-code")
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

func TestAccountLoginManualPageRelaysPostRedirectInCurrentBrowserThenAcceptsCode(t *testing.T) {
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

	// browser opener 只应打开最初的登录页。通过 fallback 页面提交 relay 后，
	// 浏览器自身必须继续跳转，不能让远端 CLI 尝试打开图形浏览器。
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

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := noRedirect.PostForm("http://"+addr+"/manual", url.Values{"code": {bridge}})
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode, string(body))
	assert.Equal(t, bridge, resp.Header.Get("Location"))
	openedURLsMu.Lock()
	openedURLSnapshot := append([]string(nil), openedURLs...)
	openedURLsMu.Unlock()
	require.Equal(t, []string{loginURL}, openedURLSnapshot)
	assert.Contains(t, stderr.String(), "Detected Pixiv authorization relay page; continuing it in the current browser.")

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

func TestAccountLoginManualPageAcceptsCallbackThroughLoopbackForwarder(t *testing.T) {
	authPath, _ := useTempPaths(t)
	remoteAddr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "forwarded-callback-code", r.Form.Get("code"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "forwarded-refresh-secret",
			"user":          map[string]any{"id": "314159", "name": "forwarded-user"},
		}))
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()
	restoreOpen := setTestOpenBrowser(t, func(string) error {
		t.Fatal("--no-open must not start a browser on the remote host")
		return nil
	})
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "auth", "login", "--no-open", "--addr", remoteAddr, "--timeout", "5s"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	waitForLoginServer(t, remoteAddr)
	forwardedAddr := startLoopbackForwarder(t, remoteAddr)

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.PostForm("http://"+forwardedAddr+"/manual", url.Values{"code": {"pixiv://account/login?code=forwarded-callback-code"}})
	require.NoError(t, err)
	page, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, string(page))
	assert.Contains(t, string(page), "Login successful")

	require.Equal(t, 0, run.waitWithin(t, 5*time.Second), stderr.String())
	store, err := auth.LoadAuthStore(authPath)
	require.NoError(t, err)
	require.Len(t, store.Accounts, 1)
	assert.Equal(t, int64(314159), store.Accounts[0].UserID)
	assert.Equal(t, "forwarded-user", store.Accounts[0].Username)
	assert.NotContains(t, stdout.String(), "forwarded-refresh-secret")
	assert.NotContains(t, stderr.String(), "forwarded-refresh-secret")
	assert.Contains(t, stderr.String(), "Browser opening is disabled; use the manual fallback page or terminal prompt.")
}

func TestAccountLoginManualPageRejectsStaleRelayWithoutOpeningIt(t *testing.T) {
	useTempPaths(t)
	addr := freeLoopbackAddr(t)
	restoreOpen := setTestOpenBrowser(t, func(string) error {
		t.Fatal("--no-open must not open a stale relay on the server")
		return nil
	})
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "auth", "login", "--no-open", "--addr", addr, "--timeout", "100ms"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	page := waitForLoginServer(t, addr)
	returnTo := pixivAuthStartURLForTest("stale-challenge")
	bridge := "https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape(returnTo)

	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {bridge}})
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
	assert.Contains(t, string(body), "Login failed")
	assert.NotContains(t, string(body), "stale-challenge")
	assert.NotContains(t, string(body), bridge)
	assert.NotContains(t, stderr.String(), bridge)
	assert.NotEmpty(t, loginURLFromPage(t, page))
	require.NotZero(t, run.waitWithin(t, 2*time.Second), stderr.String())
	assert.Empty(t, stdout.String())
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
	// 默认 transport 会读取宿主代理环境。没有显式验证代理语义的 CLI 测试必须
	// 隔离这些变量，否则本机代理会把 httptest OAuth 请求导向外部地址并造成顺序相关失败。
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		oldValue, hadValue := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		t.Cleanup(func() {
			if hadValue {
				require.NoError(t, os.Setenv(name, oldValue))
				return
			}
			require.NoError(t, os.Unsetenv(name))
		})
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// 应用数据直接位于 home/pixiv-cli；测试隔离 home 即可隔离认证、配置与日志。
	base := filepath.Join(home, constants.AppDataDirName)
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

// startLoopbackForwarder 模拟 ssh -N -L LOCAL:127.0.0.1:REMOTE。测试只接受
// loopback 端点，证明浏览器侧 POST 可以经转发回到运行 CLI 的另一端 listener。
func startLoopbackForwarder(t *testing.T, remoteAddr string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var connections sync.WaitGroup
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			incoming, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			connections.Add(1)
			go func() {
				defer connections.Done()
				defer incoming.Close()
				upstream, dialErr := net.Dial("tcp", remoteAddr)
				if dialErr != nil {
					return
				}
				defer upstream.Close()
				copied := make(chan struct{})
				go func() {
					_, _ = io.Copy(upstream, incoming)
					_ = upstream.Close()
					close(copied)
				}()
				_, _ = io.Copy(incoming, upstream)
				<-copied
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-stopped
		connections.Wait()
	})
	return listener.Addr().String()
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
