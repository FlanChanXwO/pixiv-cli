package auth

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件集中 auth owner 的纯函数行为测试与共享测试 helper：stdin token 读取、
// auth export stdout 写入、login 输入分类与 relay 校验。

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

func TestWriteAuthExportStdoutPropagatesWriterErrorSafely(t *testing.T) {
	const token = "raw-writer-error-secret"
	writer := &syntheticFailingWriter{err: errors.New(token)}
	directErr := writeAuthExportStdout(writer, []byte(token))
	require.ErrorIs(t, directErr, writer.err)
	assert.NotContains(t, directErr.Error(), token)
}

func TestWriteAuthExportStdoutShortWritesSafely(t *testing.T) {
	const token = "short-writer-secret"
	directErr := writeAuthExportStdout(syntheticShortWriter{}, []byte(token))
	require.ErrorIs(t, directErr, io.ErrShortWrite)
	assert.NotContains(t, directErr.Error(), token)
}

func TestLoginSSHTunnelCommandUsesOnlyBoundListenerAddress(t *testing.T) {
	command, err := loginSSHTunnelCommand("127.0.0.1:41871")
	require.NoError(t, err)
	assert.Equal(t, "ssh -N -L 41871:127.0.0.1:41871 USER@SERVER", command)

	_, err = loginSSHTunnelCommand("not-a-listener")
	assert.Error(t, err)
}

func TestLoginCodeFromInputOnlyPixivCallbacksMayOmitState(t *testing.T) {
	accepts := func(rawURL string) bool {
		return rawURL == "pixiv://account/login?code=app-code" || rawURL == "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=https-code"
	}
	result := loginCodeFromInput("pixiv://account/login?code=app-code", accepts)
	require.NoError(t, result.Err)
	assert.Equal(t, "pixiv://account/login?code=app-code", result.Code)

	result = loginCodeFromInput("https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=https-code", accepts)
	require.NoError(t, result.Err)
	assert.Equal(t, "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=https-code", result.Code)

	result = loginCodeFromInput("http://127.0.0.1:12345/callback?code=loopback-code", accepts)
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "does not match")

	result = loginCodeFromInput("pixiv://account/login?code=app-code&state=wrong-state", accepts)
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "does not match")
}

func TestLoginInputFromTextRelaysPostRedirect(t *testing.T) {
	returnTo := pixivAuthStartURLForTest("expected-challenge")
	bridge := "https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape(returnTo)
	var opened []string

	result := loginInputFromText(bridge, acceptsTestCallback, "expected-challenge", func(rawURL string) error {
		opened = append(opened, rawURL)
		return nil
	})

	require.NoError(t, result.Err)
	assert.True(t, result.Relayed)
	assert.Empty(t, result.Code)
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

		require.Error(t, result.Err)
		assert.True(t, result.Relayed)
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

	require.Error(t, result.Err)
	assert.True(t, result.Relayed)
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
	assert.False(t, pixivAuthStartMatchesChallenge(pixivAuthStartURLForTest("any-challenge"), ""))
}

func pixivAuthStartURLForTest(challenge string) string {
	values := url.Values{}
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("client", "pixiv-android")
	values.Set("via", "login")
	return "https://app-api.pixiv.net/web/v1/users/auth/pixiv/start?" + values.Encode()
}

func acceptsTestCallback(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || strings.TrimSpace(parsed.Query().Get("code")) == "" {
		return false
	}
	if strings.EqualFold(parsed.Scheme, "pixiv") && strings.EqualFold(parsed.Host, "account") && parsed.Path == "/login" {
		return true
	}
	return pixiv.IsOfficialOAuthCallbackURL(parsed.String())
}

type syntheticFailingWriter struct{ err error }

func (w *syntheticFailingWriter) Write([]byte) (int, error) { return 0, w.err }

type syntheticShortWriter struct{}

func (syntheticShortWriter) Write(body []byte) (int, error) { return len(body) - 1, nil }

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}

func waitForLoginServer(t *testing.T, address string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get("http://" + address + "/")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()
			return string(body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("login server did not start at %s", address)
	return ""
}

func useTempPaths(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	base := filepath.Join(home, paths.AppDataDirName)
	configPath := filepath.Join(base, "config.toml")
	t.Cleanup(paths.SetConfigFilePathForTest(configPath))
	return database.DatabasePath(base), configPath
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"DOWNLOAD_PATH", "FILENAME_TEMPLATE", "PIXIV_LOG_LEVEL", "https_proxy", "HTTPS_PROXY"} {
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
}
