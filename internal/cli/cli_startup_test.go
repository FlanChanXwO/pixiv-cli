package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/loginhelper"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

func TestAuthURLCallbackIsHiddenAndRelaysWithoutStartupSideEffects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	dir, err := files.UserDataSubdir(constants.AppDataDirName)
	requireNoError(t, err)
	requireNoError(t, os.MkdirAll(dir, constants.PrivateDirMode))
	requireNoError(t, os.WriteFile(filepath.Join(dir, "url-handler-endpoint"), []byte("http://127.0.0.1:41871/callback\n"), constants.PrivateFileMode))

	command := app{}.newAccountCommand()
	callback, _, findErr := command.Find([]string{internalURLCallbackCommand})
	requireNoError(t, findErr)
	if callback == nil || !callback.Hidden {
		t.Fatal("internal URL callback command must remain hidden")
	}

	originalCleanup := cleanupPendingWindowsUpdate
	cleanupPendingWindowsUpdate = func() error {
		t.Fatal("internal URL callback must not run startup cleanup")
		return nil
	}
	t.Cleanup(func() { cleanupPendingWindowsUpdate = originalCleanup })

	var opened string
	restoreOpen := setTestOpenBrowser(t, func(rawURL string) error {
		opened = rawURL
		return nil
	})
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", internalURLCallbackCommand, "pixiv://account/login?code=one-time-code"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || opened != "http://127.0.0.1:41871/callback#pixiv://account/login?code=one-time-code" || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("internal URL callback contract failed: code=%d opened=%q stdout_bytes=%d stderr_bytes=%d", code, opened, stdout.Len(), stderr.Len())
	}
}

// remote callback 完成后必须由隐藏 handler 显式打开一次性结果页；否则浏览器在
// pixiv:// 跳转后只会停在原授权页，用户看不到 OAuth exchange 的真实结果。
func TestAuthURLCallbackOpensRemoteFinalPage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	const secret = "remote-final-page-secret"
	const callback = "pixiv://account/login?code=one-time-code"
	const resultID = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	var relay *httptest.Server
	relay = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session":
			w.WriteHeader(http.StatusNoContent)
		case "/callback":
			w.Header().Set(loginhelper.RelayResultURLHeader, relay.URL+"/result/"+resultID)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(relay.Close)

	configPath := filepath.Join(t.TempDir(), "config.toml")
	restoreConfigPath := config.SetFilePathForTest(configPath)
	t.Cleanup(restoreConfigPath)
	requireNoError(t, config.WritePrivateFile(configPath, []byte("[login]\nrelay_target_url = \""+relay.URL+"\"\nrelay_secret = \""+secret+"\"\n")))

	var opened string
	restoreOpen := setTestOpenBrowser(t, func(rawURL string) error {
		opened = rawURL
		return nil
	})
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", internalURLCallbackCommand, callback}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || opened != relay.URL+"/result/"+resultID || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("remote URL callback final-page contract failed: code=%d opened=%q stdout_bytes=%d stderr=%q", code, opened, stdout.Len(), stderr.String())
	}
}

func TestAuthURLCallbackRejectsInputWithoutEchoingIt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	const callback = "https://example.invalid/callback?code=one-time-code"
	code := Run([]string{"pixiv", "auth", internalURLCallbackCommand, callback}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "invalid Pixiv callback URL") || strings.Contains(stderr.String(), callback) || stdout.Len() != 0 {
		t.Fatalf("internal URL callback error contract failed: code=%d stdout_bytes=%d stderr=%q", code, stdout.Len(), stderr.String())
	}
}

func TestAuthURLHandlerInstallSkipsStartupSideEffects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	originalCleanup := cleanupPendingWindowsUpdate
	cleanupPendingWindowsUpdate = func() error {
		t.Fatal("internal URL handler installer must not run startup cleanup")
		return nil
	}
	t.Cleanup(func() { cleanupPendingWindowsUpdate = originalCleanup })

	originalEnsure := ensureURLSchemeRelay
	ensureURLSchemeRelay = func(_ context.Context) error { return nil }
	t.Cleanup(func() { ensureURLSchemeRelay = originalEnsure })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", internalURLHandlerInstallCommand}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("internal URL handler installer contract failed: code=%d stdout_bytes=%d stderr=%q", code, stdout.Len(), stderr.String())
	}
}

func TestRunAuthExportSkipsPendingUpdateCleanupBeforeAnyMutation(t *testing.T) {
	authPath, _ := useTempPaths(t)
	requireNoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 7,
		Accounts:      []auth.Account{{UserID: 7, RefreshToken: "startup-export-secret"}},
	}))
	original := cleanupPendingWindowsUpdate
	t.Cleanup(func() { cleanupPendingWindowsUpdate = original })
	called := false
	cleanupPendingWindowsUpdate = func() error {
		called = true
		return errors.New("cleanup must not run")
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)

	if code != 0 || called || stdout.String() != "startup-export-secret\n" || stderr.Len() != 0 {
		t.Fatalf("auth export startup contract failed: code=%d cleanup_called=%t stdout_bytes=%d stderr_bytes=%d", code, called, stdout.Len(), stderr.Len())
	}
}

func TestRunAuthExportSkipsPendingUpdateCleanupWithLeadingRootFlags(t *testing.T) {
	authPath, _ := useTempPaths(t)
	requireNoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 7,
		Accounts:      []auth.Account{{UserID: 7, RefreshToken: "root-flag-export-secret"}},
	}))
	original := cleanupPendingWindowsUpdate
	t.Cleanup(func() { cleanupPendingWindowsUpdate = original })

	for _, test := range []struct {
		name       string
		args       []string
		wantExport bool
	}{
		{name: "help true then false", args: []string{"pixiv", "--help=true", "--help=false", "auth", "export"}, wantExport: true},
		{name: "mixed help true then false", args: []string{"pixiv", "-h=true", "--help=false", "auth", "export"}, wantExport: true},
		{name: "version true then false", args: []string{"pixiv", "--version=true", "--version=false", "auth", "export"}},
		{name: "version false", args: []string{"pixiv", "--version=false", "auth", "export"}},
		{name: "help zero", args: []string{"pixiv", "--help=0", "auth", "export"}, wantExport: true},
		{name: "short false", args: []string{"pixiv", "-h=false", "auth", "export"}, wantExport: true},
		{name: "between", args: []string{"pixiv", "auth", "--help=false", "export"}, wantExport: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			cleanupPendingWindowsUpdate = func() error {
				called = true
				return errors.New("cleanup must not run")
			}
			var stdout, stderr bytes.Buffer
			code := Run(test.args, strings.NewReader(""), &stdout, &stderr)

			if called || strings.Contains(stderr.String(), "cleanup must not run") {
				t.Fatalf("root bool flag triggered cleanup: code=%d cleanup_called=%t stdout_bytes=%d stderr_bytes=%d", code, called, stdout.Len(), stderr.Len())
			}
			if test.wantExport && (code != 0 || stdout.String() != "root-flag-export-secret\n" || stderr.Len() != 0) {
				t.Fatalf("root-flag export startup contract failed: code=%d cleanup_called=%t stdout_bytes=%d stderr_bytes=%d", code, called, stdout.Len(), stderr.Len())
			}
		})
	}
}

func TestRunRootTrueFlagsAndNonExportCommandsStillRunPendingUpdateCleanup(t *testing.T) {
	original := cleanupPendingWindowsUpdate
	t.Cleanup(func() { cleanupPendingWindowsUpdate = original })

	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "help true", args: []string{"pixiv", "--help=true", "auth", "export"}},
		{name: "help false then true", args: []string{"pixiv", "--help=false", "--help=true", "auth", "export"}},
		{name: "mixed help false then true", args: []string{"pixiv", "--help=false", "-h=true", "auth", "export"}},
		{name: "version one", args: []string{"pixiv", "--version=1", "auth", "export"}},
		{name: "version false then true", args: []string{"pixiv", "--version=false", "--version=true", "auth", "export"}},
		{name: "version command", args: []string{"pixiv", "version", "--json"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			cleanupPendingWindowsUpdate = func() error {
				called = true
				return errors.New("expected cleanup failure")
			}
			var stdout, stderr bytes.Buffer
			code := Run(test.args, strings.NewReader(""), &stdout, &stderr)

			if code != 1 || !called || !strings.Contains(stderr.String(), "clean pending update: expected cleanup failure") {
				t.Fatalf("normal startup cleanup contract failed: code=%d cleanup_called=%t stdout_bytes=%d stderr_bytes=%d", code, called, stdout.Len(), stderr.Len())
			}
		})
	}
}

func TestRunInvalidRootBoolKeepsCobraParseError(t *testing.T) {
	original := cleanupPendingWindowsUpdate
	t.Cleanup(func() { cleanupPendingWindowsUpdate = original })
	called := false
	cleanupPendingWindowsUpdate = func() error {
		called = true
		return nil
	}
	var stdout, stderr bytes.Buffer

	code := Run([]string{"pixiv", "--help=not-bool", "--help=false", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)

	if code != 1 || !called || !strings.Contains(stderr.String(), "invalid argument") {
		t.Fatalf("invalid root bool did not preserve Cobra error: code=%d cleanup_called=%t stdout_bytes=%d stderr_bytes=%d", code, called, stdout.Len(), stderr.Len())
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunStopsWhenPendingUpdateCleanupFails(t *testing.T) {
	original := cleanupPendingWindowsUpdate
	t.Cleanup(func() { cleanupPendingWindowsUpdate = original })
	cleanupPendingWindowsUpdate = func() error { return errors.New("pending backup cannot be removed") }

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "version", "--json"}, strings.NewReader(""), &stdout, &stderr)

	if code != 1 {
		t.Fatalf("Run exit code = %d, want 1", code)
	}
	if got := stderr.String(); !strings.Contains(got, "clean pending update: pending backup cannot be removed") {
		t.Fatalf("stderr = %q, want cleanup error", got)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestRunContinuesAfterPendingUpdateCleanup(t *testing.T) {
	original := cleanupPendingWindowsUpdate
	t.Cleanup(func() { cleanupPendingWindowsUpdate = original })
	called := false
	cleanupPendingWindowsUpdate = func() error {
		called = true
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "version", "--json"}, strings.NewReader(""), &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run exit code = %d, stderr = %q", code, stderr.String())
	}
	if !called {
		t.Fatal("pending update cleanup was not called")
	}
}
