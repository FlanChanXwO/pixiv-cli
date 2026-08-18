package auth_test

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	auth "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件集中 auth owner 的纯函数行为测试与共享测试 helper：stdin token 读取、
// auth export stdout 写入、login 输入分类、relay 校验与登录页渲染。行为测试只
// 通过导出的纯函数观察实现，不从根 package 借 bridge。

func TestWriteLoginFinalPageCentersAndHidesSensitiveFailure(t *testing.T) {
	t.Parallel()
	form := httptest.NewRecorder()
	auth.WriteLoginForm(form, "https://app-api.pixiv.net/web/v1/login?state=test")
	if !strings.Contains(form.Body.String(), "<title>pixiv-cli</title>") {
		t.Fatalf("manual page title = %s", form.Body.String())
	}

	callback := httptest.NewRecorder()
	auth.WriteLoginCallbackRelayPage(callback)
	if !strings.Contains(callback.Body.String(), "<title>pixiv-cli</title>") || strings.Contains(callback.Body.String(), "document.title = \"Login failed\"") {
		t.Fatalf("callback page must retain pixiv-cli title: %s", callback.Body.String())
	}

	rec := httptest.NewRecorder()
	auth.WriteLoginFinalPage(rec, true)
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(body, "text-align:center") || !strings.Contains(body, "display:flex") {
		t.Fatalf("success page not centered: %s", body)
	}
	if !strings.Contains(body, "Login successful") || !strings.Contains(body, "<h1>Login successful</h1>") || !strings.Contains(body, `<html lang="en">`) || !strings.Contains(body, "<title>pixiv-cli</title>") {
		t.Fatalf("success title missing: %s", body)
	}

	rec = httptest.NewRecorder()
	auth.WriteLoginFinalPage(rec, false)
	body = rec.Body.String()
	if rec.Code != 400 {
		t.Fatalf("fail status=%d", rec.Code)
	}
	if !strings.Contains(body, "text-align:center") || !strings.Contains(body, "display:flex") {
		t.Fatalf("failure page not centered: %s", body)
	}
	if !strings.Contains(body, "Login failed") || !strings.Contains(body, "<title>pixiv-cli</title>") {
		t.Fatalf("failure title missing: %s", body)
	}
	for _, page := range []string{form.Body.String(), callback.Body.String(), body} {
		if strings.Contains(strings.ToLower(page), `rel="icon"`) {
			t.Fatalf("login page must not override the browser favicon: %s", page)
		}
	}
	// 失败页不得回显敏感片段
	for _, secret := range []string{"token", "refresh", "Bearer", "code=", "password", "secret"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(secret)) {
			t.Fatalf("failure page leaked %q: %s", secret, body)
		}
	}
}

func TestAccountImportStdinRemovesOnlyOneTrailingLF(t *testing.T) {
	for name, input := range map[string]string{
		"LF":   "  opaque stdin token  \n",
		"CRLF": "  opaque stdin token  \r\n",
	} {
		t.Run(name, func(t *testing.T) {
			token, err := auth.ReadRefreshTokenInput(strings.NewReader(input))
			require.NoError(t, err)
			assert.Equal(t, "  opaque stdin token  ", token)
		})
	}
}

func TestAccountImportStdinRejectsAdditionalNewline(t *testing.T) {
	_, err := auth.ReadRefreshTokenInput(strings.NewReader("first-line\nsecond-line\n"))
	require.EqualError(t, err, "refresh token input must contain exactly one line")
}

func TestAccountImportStdinRejectsEmptyInput(t *testing.T) {
	_, err := auth.ReadRefreshTokenInput(strings.NewReader(""))
	require.EqualError(t, err, "refresh token cannot be empty")
}

func TestWriteAuthExportStdoutPropagatesWriterErrorSafely(t *testing.T) {
	const token = "raw-writer-error-secret"
	writer := &syntheticFailingWriter{err: errors.New(token)}
	directErr := auth.WriteAuthExportStdout(writer, []byte(token))
	require.ErrorIs(t, directErr, writer.err)
	assert.NotContains(t, directErr.Error(), token)
}

func TestWriteAuthExportStdoutShortWritesSafely(t *testing.T) {
	const token = "short-writer-secret"
	directErr := auth.WriteAuthExportStdout(syntheticShortWriter{}, []byte(token))
	require.ErrorIs(t, directErr, io.ErrShortWrite)
	assert.NotContains(t, directErr.Error(), token)
}

func TestLoginSSHTunnelCommandUsesOnlyBoundListenerAddress(t *testing.T) {
	command, err := auth.LoginSSHTunnelCommand("127.0.0.1:41871")
	require.NoError(t, err)
	assert.Equal(t, "ssh -N -L 41871:127.0.0.1:41871 USER@SERVER", command)

	_, err = auth.LoginSSHTunnelCommand("not-a-listener")
	assert.Error(t, err)
}

func TestLoginCodeFromInputOnlyPixivCallbacksMayOmitState(t *testing.T) {
	accepts := func(rawURL string) bool {
		return rawURL == "pixiv://account/login?code=app-code" || rawURL == "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=https-code"
	}
	result := auth.LoginCodeFromInput("pixiv://account/login?code=app-code", accepts)
	require.NoError(t, result.Err)
	assert.Equal(t, "pixiv://account/login?code=app-code", result.Code)

	result = auth.LoginCodeFromInput("https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=https-code", accepts)
	require.NoError(t, result.Err)
	assert.Equal(t, "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=https-code", result.Code)

	result = auth.LoginCodeFromInput("http://127.0.0.1:12345/callback?code=loopback-code", accepts)
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "does not match")

	result = auth.LoginCodeFromInput("pixiv://account/login?code=app-code&state=wrong-state", accepts)
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "does not match")
}

func TestLoginInputFromTextRelaysPostRedirect(t *testing.T) {
	returnTo := pixivAuthStartURLForTest("expected-challenge")
	bridge := "https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape(returnTo)
	var opened []string

	result := auth.LoginInputFromText(bridge, acceptsTestCallback, "expected-challenge", func(rawURL string) error {
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
		result := auth.LoginInputFromText(input, acceptsTestCallback, "expected-challenge", func(rawURL string) error {
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

	result := auth.LoginInputFromText(bridge, acceptsTestCallback, "expected-challenge", func(rawURL string) error {
		opened = append(opened, rawURL)
		return nil
	})

	require.Error(t, result.Err)
	assert.True(t, result.Relayed)
	assert.Empty(t, opened)
}

func TestPixivPostRedirectReturnToAcceptsOnlyPixivStartURL(t *testing.T) {
	returnTo := "https://app-api.pixiv.net/web/v1/users/auth/pixiv/start?code_challenge=challenge&client=pixiv-android"
	actual, ok := auth.PixivPostRedirectReturnTo("https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape(returnTo))
	require.True(t, ok)
	assert.Equal(t, returnTo, actual)

	_, ok = auth.PixivPostRedirectReturnTo("https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape("https://example.test/web/v1/users/auth/pixiv/start"))
	assert.False(t, ok)

	_, ok = auth.PixivPostRedirectReturnTo("https://accounts.pixiv.net/post-redirect?return_to=" + url.QueryEscape("https://app-api.pixiv.net/not-start"))
	assert.False(t, ok)
}

func TestPixivAuthStartMatchesChallenge(t *testing.T) {
	assert.True(t, auth.PixivAuthStartMatchesChallenge(pixivAuthStartURLForTest("current-challenge"), "current-challenge"))
	assert.False(t, auth.PixivAuthStartMatchesChallenge(pixivAuthStartURLForTest("stale-challenge"), "current-challenge"))
	assert.True(t, auth.PixivAuthStartMatchesChallenge(pixivAuthStartURLForTest("any-challenge"), ""))
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
	return err == nil && auth.IsBrowserCallbackURL(parsed) && strings.TrimSpace(parsed.Query().Get("code")) != ""
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
