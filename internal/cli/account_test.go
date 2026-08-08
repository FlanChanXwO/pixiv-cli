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

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountImportAcceptsPositionalRefreshToken(t *testing.T) {
	useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"opaque/import-token": {userID: 333, username: "import-user"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", "opaque/import-token"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "added uid:333\nusername:import-user\n", stdout.String())
	assert.Empty(t, stderr.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var out accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Len(t, out.Accounts, 1)
	assert.Equal(t, int64(333), out.Accounts[0].UserID)
	assert.Equal(t, "import-user", out.Accounts[0].Username)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "import", "opaque/import-token"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "updated uid:333\nusername:import-user\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestAccountImportJSONDoesNotEchoInputOrRotatedToken(t *testing.T) {
	useTempPaths(t)
	const inputToken = "input-token-canary"
	const rotatedToken = "rotated-token-canary"
	store := &fakeAccountStore{}
	store.importResult = func(_ context.Context, token string, _ bool) (pixivapp.Account, error) {
		assert.Equal(t, inputToken, token)
		return pixivapp.Account{UserID: 335, Username: "redacted-user"}, nil
	}
	setTestAccountStore(t, store)

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
	useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"main/token":  {userID: 111, username: "alice"},
		"other-token": {userID: 222, username: "bob"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", "main/token"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "import", "other-token"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "use", "222"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

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

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	out = accountListOut{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, int64(111), out.DefaultUserID)
	require.Len(t, out.Accounts, 1)
	assert.Equal(t, int64(111), out.Accounts[0].UserID)
}

func TestAccountImportReadsTokenFromStdin(t *testing.T) {
	useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"stdin-token": {userID: 333, username: "stdin-user"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import"}, strings.NewReader("stdin-token\n"), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "added uid:333\nusername:stdin-user\n", stdout.String())
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var out accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Len(t, out.Accounts, 1)
	assert.Equal(t, int64(333), out.Accounts[0].UserID)
	assert.Equal(t, "stdin-user", out.Accounts[0].Username)
}

func TestAccountRefreshForcesPremiumStatusAndPrintsSafeAccountSummary(t *testing.T) {
	useTempPaths(t)
	premium := false
	setTestAccountStore(t, &fakeAccountStore{
		accounts: []pixivapp.Account{{UserID: 123, Username: "stored-user", Default: true, Premium: &premium}},
		refreshResults: map[int64]pixivapp.Account{
			123: {UserID: 123, Username: "refreshed-user", Default: true, Premium: &premium},
		},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "refresh", "123", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.JSONEq(t, `{"accounts":[{"user_id":123,"username":"refreshed-user","default":true,"has_token":true,"premium_status":false}]}`, stdout.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "refresh", "123"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "✓ refreshed uid:123 premium:no\n", stdout.String())
}

func TestAccountListUsesCredentialSymbolInsteadOfTokenBoolean(t *testing.T) {
	useTempPaths(t)
	setTestAccountStore(t, &fakeAccountStore{
		accounts: []pixivapp.Account{
			{UserID: 123, Username: "saved", Default: true},
			{UserID: 456, Username: "second"},
		},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "list"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "* ✓ uid:123 username:saved\n  ✓ uid:456 username:second\n", stdout.String())
	assert.NotContains(t, stdout.String(), "token:")
}

func TestAccountRefreshAllRejectsEmptyAccountStore(t *testing.T) {
	useTempPaths(t)
	setTestAccountStore(t, &fakeAccountStore{accounts: []pixivapp.Account{}})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "refresh", "--all"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "no accounts")
}

func TestAccountImportAcceptsProxyFlagWithoutRoutingThroughNetwork(t *testing.T) {
	useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"input-token": {userID: 444, username: "proxy-user"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", "input-token", "--proxy", "http://127.0.0.1:1"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "added uid:444\nusername:proxy-user\n", stdout.String())
}

func TestAccountImportNoProxyFlagStillImports(t *testing.T) {
	useTempPaths(t)
	setTestAuthClientFactory(t, map[string]authIdentity{
		"input-token": {userID: 445, username: "no-proxy-user"},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", "input-token", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "added uid:445\nusername:no-proxy-user\n", stdout.String())
}

func TestAccountCheckNoProxyFlagValidatesStoredAccount(t *testing.T) {
	useTempPaths(t)
	setTestAccountStore(t, &fakeAccountStore{
		accounts: []pixivapp.Account{{UserID: 444, Username: "check-user", Default: true}},
		checkResults: map[int64]pixivapp.Account{
			444: {UserID: 444, Username: "check-user", Default: true},
		},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "check", "--no-proxy"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "token ok, uid:444\nusername:check-user\n", stdout.String())
}

func TestAccountCheckUsesEnvironmentTokenWithoutChangingDefaultAccount(t *testing.T) {
	useTempPaths(t)
	t.Setenv("PIXIV_REFRESH_TOKEN", "environment-token")
	store := &fakeAccountStore{
		accounts: []pixivapp.Account{{UserID: 444, Username: "stored", Default: true}},
	}
	store.checkRefreshToken = func(_ context.Context, token string) (pixivapp.Account, error) {
		assert.Equal(t, "environment-token", token)
		return pixivapp.Account{UserID: 555, Username: "environment-user"}, nil
	}
	setTestAccountStore(t, store)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "check", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var out accountOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Equal(t, accountOut{UserID: 555, Username: "environment-user", HasToken: true}, out)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var listed accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &listed))
	require.Len(t, listed.Accounts, 1)
	assert.Equal(t, int64(444), listed.Accounts[0].UserID)
}

func TestAccountCheckProxyFlagStillValidatesStoredAccount(t *testing.T) {
	useTempPaths(t)
	setTestAccountStore(t, &fakeAccountStore{
		accounts: []pixivapp.Account{{UserID: 446, Username: "check-proxy-user", Default: true}},
		checkResults: map[int64]pixivapp.Account{
			446: {UserID: 446, Username: "check-proxy-user", Default: true},
		},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "check", "--proxy", "http://127.0.0.1:1"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "token ok, uid:446\nusername:check-proxy-user\n", stdout.String())
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

func TestDataCommandFlagsDoNotExposeCredentialSelection(t *testing.T) {
	a := app{}
	commonCommand := &cobra.Command{}
	var commonOptions commandOptions
	a.bindCommonFlags(commonCommand, &commonOptions)
	assert.Nil(t, commonCommand.Flags().Lookup("refresh-token"))
	assert.Nil(t, commonCommand.Flags().Lookup("uid"))

	importCommand := a.newAccountImportCommand()
	assert.Nil(t, importCommand.Flags().Lookup("token"))
}

func TestAuthExportPrintsOnlyDefaultStoredRefreshToken(t *testing.T) {
	authPath, _ := useTempPaths(t)
	t.Setenv("PIXIV_REFRESH_TOKEN", "environment-token-must-be-ignored")
	require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 444,
		Accounts:      []testAuthAccount{{UserID: 444, Username: "stored", RefreshToken: "opaque/default-token"}},
	}))
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "opaque/default-token\n", stdout.String())
	assert.Empty(t, stderr.String())
	_, err := os.Stat(filepath.Join(filepath.Dir(authPath), "auth.json"))
	assert.ErrorIs(t, err, os.ErrNotExist, "the runtime must not create a legacy secret store")
}

func TestAuthExportRawTokenPropagatesStdoutWriterErrorSafely(t *testing.T) {
	authPath, _ := useTempPaths(t)
	const token = "raw-writer-error-secret"
	require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 444,
		Accounts:      []testAuthAccount{{UserID: 444, RefreshToken: token}},
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
	require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 444,
		Accounts:      []testAuthAccount{{UserID: 444, RefreshToken: token}},
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
	require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 444,
		Accounts: []testAuthAccount{
			{UserID: 444, Username: "first", RefreshToken: "first-export-secret"},
			{UserID: 555, Username: "second", RefreshToken: "second-export-secret"},
		},
	}))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "export", "--all"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Empty(t, stderr.String())
	bundlePath := filepath.Join(t.TempDir(), "auth-export.json")
	require.NoError(t, os.WriteFile(bundlePath, stdout.Bytes(), 0o600))
	// Export reads the authoritative DB. Start the bundle restore from a fresh DB.
	require.NoError(t, os.Remove(authdb.DatabasePath(filepath.Dir(authPath))))

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "import", "--file", bundlePath}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.Equal(t, "added uid:444\nadded uid:555\ndefault uid: 444\n", stdout.String())
	assert.Empty(t, stderr.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var out accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Len(t, out.Accounts, 2)
	ids := []int64{out.Accounts[0].UserID, out.Accounts[1].UserID}
	assert.ElementsMatch(t, []int64{444, 555}, ids)
}

func TestAuthImportBundleFromStdinPrintsSafeJSONReport(t *testing.T) {
	authPath, _ := useTempPaths(t)
	require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 888,
		Accounts:      []testAuthAccount{{UserID: 888, Username: "before", RefreshToken: "old-local-secret"}},
	}))
	const bundle = `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":777,"accounts":[{"user_id":777,"username":"new","refresh_token":"stdin-bundle-secret"},{"user_id":888,"username":"updated","refresh_token":"updated-bundle-secret"}]}`

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import", "--file", "-", "--json"}, strings.NewReader(bundle), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	assert.JSONEq(t, `{"accounts":[{"user_id":777,"username":"new","status":"added"},{"user_id":888,"username":"updated","status":"added"}],"default_user_id":777}`, stdout.String())
	assert.NotContains(t, stdout.String(), "stdin-bundle-secret")
	assert.NotContains(t, stderr.String(), "stdin-bundle-secret")
	assert.NotContains(t, stdout.String(), "updated-bundle-secret")
	assert.NotContains(t, stderr.String(), "updated-bundle-secret")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var out accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Len(t, out.Accounts, 2)
	ids := []int64{out.Accounts[0].UserID, out.Accounts[1].UserID}
	assert.ElementsMatch(t, []int64{777, 888}, ids)
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
	require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 888,
		Accounts:      []testAuthAccount{{UserID: 888, Username: "exported", RefreshToken: "output-secret-canary"}},
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
	var bundle pixivapp.AuthExportBundle
	require.NoError(t, json.Unmarshal(exportedBody, &bundle))
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

func TestAuthExportIgnoresLegacyLoggingConfiguration(t *testing.T) {
	authPath, configPath := useTempPaths(t)
	t.Setenv("PIXIV_LOG_LEVEL", "loud")
	require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 444,
		Accounts:      []testAuthAccount{{UserID: 444, RefreshToken: "synthetic-invalid-refresh-token"}},
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
	require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 444,
		Accounts: []testAuthAccount{
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
			wantCode := 1
			if name == "json flag" || name == "proxy flag" || name == "no proxy flag" {
				wantCode = 2
			}
			assert.Equal(t, wantCode, code)
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
			wantError: "pixiv:auth: unauthorized: no pixiv account is authenticated",
		},
		{
			name: "explicit account missing",
			args: []string{"pixiv", "auth", "export", "999"},
			prepare: func(t *testing.T, authPath string) {
				require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{
					DefaultUserID: 444,
					Accounts:      []testAuthAccount{{UserID: 444, RefreshToken: storedTokenCanary}},
				}))
			},
			wantError: "account uid 999 not found",
			forbidden: storedTokenCanary,
		},
		{
			name: "legacy auth json is ignored",
			args: []string{"pixiv", "auth", "export"},
			prepare: func(t *testing.T, authPath string) {
				legacyPath := filepath.Join(filepath.Dir(authPath), "auth.json")
				require.NoError(t, config.WritePrivateFile(legacyPath, []byte(`{"accounts":[`+malformedCanary+`]}`)))
			},
			wantError: "pixiv:auth: unauthorized: no pixiv account is authenticated",
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

func TestAccountPromptFlows(t *testing.T) {
	useTempPaths(t)
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
	assert.Contains(t, stdout.String(), "added uid:444")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var out accountListOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	require.Len(t, out.Accounts, 1)
	assert.Equal(t, int64(444), out.Accounts[0].UserID)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "use"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "remove"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	out = accountListOut{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.Empty(t, out.Accounts)
}

func TestAccountRemovePromptCancelKeepsData(t *testing.T) {
	authPath, _ := useTempPaths(t)
	require.NoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 555,
		Accounts:      []testAuthAccount{{UserID: 555, Username: "kept-user", RefreshToken: "main-token"}},
	}))
	setPromptStub(t, promptStub{
		selects:  []string{"555 kept-user"},
		confirms: []bool{false},
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "remove"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	db, err := authdb.Open(filepath.Dir(authPath))
	require.NoError(t, err)
	defer db.Close()
	accounts, err := db.ListPixiv(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	assert.Equal(t, int64(555), accounts[0].UserID)
}

func TestLoginSSHTunnelCommandUsesOnlyBoundListenerAddress(t *testing.T) {
	command, err := loginSSHTunnelCommand("127.0.0.1:41871")
	require.NoError(t, err)
	assert.Equal(t, "ssh -N -L 41871:127.0.0.1:41871 USER@SERVER", command)

	_, err = loginSSHTunnelCommand("not-a-listener")
	assert.Error(t, err)
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
	assert.NotEmpty(t, loginURLFromPage(t, page))
	code := run.waitWithin(t, 2*time.Second)
	assert.NotContains(t, stderr.String(), bridge)
	require.NotZero(t, code, stderr.String())
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

func pixivAuthStartURLForTest(challenge string) string {
	values := url.Values{}
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("client", "pixiv-android")
	values.Set("via", "login")
	return "https://app-api.pixiv.net/web/v1/users/auth/pixiv/start?" + values.Encode()
}

func setTestOpenBrowser(t *testing.T, opener func(string) error) func() {
	t.Helper()
	return setOpenBrowserForTest(opener)
}

func setTestPublicSDKFactoryWithHTTPClient(t *testing.T, oauthBaseURL, appAPIBaseURL, webAPIBaseURL string, resourcePolicy pixiv.ResourcePolicy, newHTTPClient func(string) (*http.Client, error), observe func(pixivapp.SDKClientRequest)) {
	t.Helper()
	old := newCLIServices
	newCLIServices = func() (*bootstrap.Runtime, error) {
		services := newTestRuntime(t)
		pixivService, _ := services.Account.Pixiv.(*pixivapp.Service)
		services.SDK.NewClient = func(request pixivapp.SDKClientRequest) (pixivapp.ClientSet, error) {
			if observe != nil {
				observe(request)
			}
			rewrites := []hostRewrite{
				{host: "oauth.secure.pixiv.net", base: oauthBaseURL},
				{host: "app-api.pixiv.net", base: appAPIBaseURL},
				{host: "i.pximg.net", base: webAPIBaseURL},
			}
			options := pixiv.Options{ResourcePolicy: resourcePolicy}
			if request.HTTPSProxyOverride != nil {
				httpClient, err := newHTTPClient(*request.HTTPSProxyOverride)
				if err != nil {
					return pixivapp.ClientSet{}, err
				}
				options.HTTPClient = rewriteAndProxyHTTPClient(rewrites, httpClient.Transport)
			} else {
				options.HTTPClient = rewriteAndProxyHTTPClient(rewrites, nil)
			}
			ctx := context.Background()
			refreshToken := request.RefreshToken
			if refreshToken == "" {
				appDataDir := filepath.Join(os.Getenv("HOME"), filesystem.AppDataDirName)
				db, err := authdb.Open(appDataDir)
				if err != nil {
					return pixivapp.ClientSet{}, err
				}
				defer db.Close()
				accounts, err := db.ListPixiv(ctx)
				if err != nil {
					return pixivapp.ClientSet{}, err
				}
				if len(accounts) > 0 {
					refreshToken = string(accounts[0].RefreshToken)
				}
			}
			if refreshToken == "" {
				return pixivapp.ClientSet{}, errors.New("no refresh token available for test SDK client")
			}
			client, _, err := pixiv.OpenWith(ctx, refreshToken, options)
			if err != nil {
				return pixivapp.ClientSet{}, err
			}
			return pixivapp.NewPixivSDKClients(client, pixivService), nil
		}
		// 测试工厂必须走上面的本地 httptest transport；生产账号池 closure 会直接
		// 构造真实 OpenWith client，不能在此 helper 中沿用。这里用同一 factory 的
		// 非池化重放，保证 list/download 命令不会因 nil RunPooled 而 panic。
		services.SDK.RunPooled = func(ctx context.Context, request pixivapp.SDKClientRequest, attempt func(context.Context, pixivapp.ClientSet) (bool, error)) error {
			client, err := services.SDK.NewClient(request)
			if err != nil {
				return err
			}
			_, err = attempt(ctx, client)
			return err
		}
		// Login 持有 SDKService 值；重新装配后必须同步替换它。
		services.Login.SDK = services.SDK
		return services, nil
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

// fakeAccountStore 实现 pixivapp.AccountStore；只覆写本组测试经过的方法。
type fakeAccountStore struct {
	accounts          []pixivapp.Account
	refreshResults    map[int64]pixivapp.Account
	checkResults      map[int64]pixivapp.Account
	checkRefreshToken func(context.Context, string) (pixivapp.Account, error)
	importResult      func(context.Context, string, bool) (pixivapp.Account, error)
	exportTokens      map[int64]string
	refreshErr        error
}

func (f *fakeAccountStore) ImportAccount(ctx context.Context, token string, setDefault bool) (pixivapp.Account, error) {
	var account pixivapp.Account
	var err error
	if f.importResult != nil {
		account, err = f.importResult(ctx, token, setDefault)
	} else {
		err = errors.New("unexpected account import")
	}
	if err != nil {
		return pixivapp.Account{}, err
	}
	f.accounts = append(f.accounts, account)
	return account, nil
}

func (f *fakeAccountStore) ListAccounts(context.Context) ([]pixivapp.Account, error) {
	if f.accounts == nil {
		return []pixivapp.Account{}, nil
	}
	return f.accounts, nil
}

func (f *fakeAccountStore) UseAccount(_ context.Context, userID int64) error {
	for i := range f.accounts {
		f.accounts[i].Default = f.accounts[i].UserID == userID
	}
	return nil
}

func (f *fakeAccountStore) RemoveAccount(_ context.Context, userID int64) error {
	wasDefault := false
	for i, account := range f.accounts {
		if account.UserID == userID {
			wasDefault = account.Default
			f.accounts = append(f.accounts[:i], f.accounts[i+1:]...)
			break
		}
	}
	if wasDefault && len(f.accounts) > 0 {
		f.accounts[0].Default = true
	}
	return nil
}

func (f *fakeAccountStore) CheckAccount(_ context.Context, userID int64) (pixivapp.Account, error) {
	if account, ok := f.checkResults[userID]; ok {
		return account, nil
	}
	return pixivapp.Account{}, errors.New("account not found")
}

func (f *fakeAccountStore) CheckRefreshToken(ctx context.Context, token string) (pixivapp.Account, error) {
	if f.checkRefreshToken != nil {
		return f.checkRefreshToken(ctx, token)
	}
	return pixivapp.Account{}, errors.New("unexpected refresh token check")
}

func (f *fakeAccountStore) ExportRefreshToken(_ context.Context, userID int64) (string, error) {
	if token, ok := f.exportTokens[userID]; ok {
		return token, nil
	}
	return "", errors.New("account not found")
}

func (f *fakeAccountStore) RefreshAccount(_ context.Context, userID int64) (pixivapp.Account, error) {
	if account, ok := f.refreshResults[userID]; ok {
		return account, nil
	}
	if f.refreshErr != nil {
		return pixivapp.Account{}, f.refreshErr
	}
	return pixivapp.Account{}, errors.New("account not found")
}

func (f *fakeAccountStore) CurrentUser(context.Context) (*pixivapp.Account, error) {
	for i := range f.accounts {
		if f.accounts[i].Default {
			account := f.accounts[i]
			return &account, nil
		}
	}
	if len(f.accounts) > 0 {
		account := f.accounts[0]
		return &account, nil
	}
	return nil, errors.New("no accounts")
}

func (f *fakeAccountStore) RestoreAccount(ctx context.Context, account pixivapp.Account, token string, setDefault bool) error {
	f.accounts = append(f.accounts, account)
	return nil
}

func (f *fakeAccountStore) AccountsWithTokens(context.Context) ([]pixivapp.AccountWithToken, error) {
	return nil, nil
}

// setTestAccountStore 让 CLI 账号命令直接观察 AccountStore，而不触发 OAuth 网络调用。
func setTestAccountStore(t *testing.T, store pixivapp.AccountStore) {
	t.Helper()
	old := newCLIServices
	newCLIServices = func() (*bootstrap.Runtime, error) {
		services := newTestRuntime(t)
		services.Account.Pixiv = store
		return services, nil
	}
	t.Cleanup(func() { newCLIServices = old })
}

func setTestAuthClientFactory(t *testing.T, identities map[string]authIdentity) {
	t.Helper()
	store := &fakeAccountStore{}
	store.importResult = func(_ context.Context, token string, _ bool) (pixivapp.Account, error) {
		identity, ok := identities[token]
		if !ok {
			return pixivapp.Account{}, fmt.Errorf("unexpected refresh token %q", token)
		}
		return pixivapp.Account{UserID: identity.userID, Username: identity.username}, nil
	}
	store.checkResults = make(map[int64]pixivapp.Account)
	for _, identity := range identities {
		store.checkResults[identity.userID] = pixivapp.Account{UserID: identity.userID, Username: identity.username}
	}
	setTestAccountStore(t, store)
}

type promptStub struct {
	inputs      []string
	inputOutput string
	secrets     []string
	selects     []string
	confirms    []bool
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
		if stub.inputOutput != "" {
			_, _ = fmt.Fprint(a.out, stub.inputOutput)
		}
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
	// 应用数据直接位于 home/.pixiv-cli；测试隔离 home 即可隔离认证、配置与日志。
	base := filepath.Join(home, filesystem.AppDataDirName)
	databasePath := authdb.DatabasePath(base)
	configPath := filepath.Join(base, "config.toml")
	t.Cleanup(config.SetFilePathForTest(configPath))
	return databasePath, configPath
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
