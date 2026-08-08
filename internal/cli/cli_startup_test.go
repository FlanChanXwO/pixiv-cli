package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/auth/loginhelper"
	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
)

func TestAuthURLCallbackIsHiddenAndRelaysWithoutStartupSideEffects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	dir, err := filesystem.UserDataSubdir(filesystem.AppDataDirName)
	requireNoError(t, err)
	requireNoError(t, os.MkdirAll(dir, filesystem.PrivateDirMode))
	requireNoError(t, os.WriteFile(filepath.Join(dir, "url-handler-endpoint"), []byte("http://127.0.0.1:41871/callback\n"), filesystem.PrivateFileMode))

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
func TestAuthURLCallbackStartsOneTimeHandoffInBrowser(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	const session = "handoff-browser-session"
	const proof = "handoff-browser-proof"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start/" + session:
			_, _ = w.Write([]byte(`{"authorization_url":"https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=handoff-browser-challenge&state=handoff-browser"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	var opened string
	restoreOpen := setTestOpenBrowser(t, func(rawURL string) error {
		opened = rawURL
		return nil
	})
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	raw := "pixiv://account/remote-login?origin=" + url.QueryEscape(server.URL) + "&session=" + session + "&access=" + proof
	code := Run([]string{"pixiv", "auth", internalURLCallbackCommand, raw}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || opened != "https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=handoff-browser-challenge&state=handoff-browser" || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("handoff login start handler contract failed: code=%d opened=%q stdout_bytes=%d stderr=%q", code, opened, stdout.Len(), stderr.String())
	}
}

func TestAuthURLCallbackBrowserFailureClearsCurrentHandoff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	const session = "handoff-browser-failure"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/start/"+session {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"authorization_url":"https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=challenge&state=handoff-browser"}`))
	}))
	t.Cleanup(server.Close)

	restoreOpen := setTestOpenBrowser(t, func(string) error { return errors.New("browser unavailable") })
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	raw := "pixiv://account/remote-login?origin=" + url.QueryEscape(server.URL) + "&session=" + session + "&access=handoff-browser-proof"
	code := Run([]string{"pixiv", "auth", internalURLCallbackCommand, raw}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "could not open Pixiv authorization page") || stdout.Len() != 0 {
		t.Fatalf("browser failure contract changed: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	_, err := loginhelper.ForwardActiveRemoteLoginCallback(context.Background(), "pixiv://account/login?code=must-delegate")
	if !errors.Is(err, loginhelper.ErrNoActiveRemoteLogin) {
		t.Fatalf("browser failure left an active remote handoff: %v", err)
	}
}

func TestAuthURLCallbackRejectsInputWithoutEchoingIt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	const callback = "https://example.invalid/callback?code=one-time-code"
	code := Run([]string{"pixiv", "auth", internalURLCallbackCommand, callback}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "invalid Pixiv login link") || strings.Contains(stderr.String(), callback) || stdout.Len() != 0 {
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

func TestNormalCLIInvocationEnsuresPersistentHandlerWithoutBlockingCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	originalSupported := automaticPersistentHandlerSupported
	originalEnsure := ensureURLSchemeRelay
	automaticPersistentHandlerSupported = func() bool { return true }
	calls := 0
	ensureURLSchemeRelay = func(context.Context) error {
		calls++
		return errors.New("desktop integration unavailable")
	}
	t.Cleanup(func() {
		automaticPersistentHandlerSupported = originalSupported
		ensureURLSchemeRelay = originalEnsure
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "version"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || calls != 1 || !strings.Contains(stderr.String(), "persistent pixiv:// callback handler was not initialized") {
		t.Fatalf("normal command did not retain handler setup as non-blocking side effect: code=%d calls=%d stdout=%q stderr=%q", code, calls, stdout.String(), stderr.String())
	}
}

func TestSecretExportAndInternalCallbackSkipAutomaticHandlerSetup(t *testing.T) {
	authPath, _ := useTempPaths(t)
	requireNoError(t, saveTestAuthStore(t, authPath, testAuthStore{DefaultUserID: 9, Accounts: []testAuthAccount{{UserID: 9, RefreshToken: "auto-handler-must-not-touch-export"}}}))
	originalSupported := automaticPersistentHandlerSupported
	originalEnsure := ensureURLSchemeRelay
	automaticPersistentHandlerSupported = func() bool { return true }
	ensureURLSchemeRelay = func(context.Context) error {
		t.Fatal("protected invocation must not install a persistent URL handler")
		return nil
	}
	t.Cleanup(func() {
		automaticPersistentHandlerSupported = originalSupported
		ensureURLSchemeRelay = originalEnsure
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stdout.String() != "auto-handler-must-not-touch-export\n" || stderr.Len() != 0 {
		t.Fatalf("auth export changed by handler setup: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunAuthExportSkipsPendingUpdateCleanupBeforeAnyMutation(t *testing.T) {
	authPath, _ := useTempPaths(t)
	requireNoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 7,
		Accounts:      []testAuthAccount{{UserID: 7, RefreshToken: "startup-export-secret"}},
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
	requireNoError(t, saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 7,
		Accounts:      []testAuthAccount{{UserID: 7, RefreshToken: "root-flag-export-secret"}},
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

func TestRunParsedCommandRunsPendingUpdateCleanup(t *testing.T) {
	original := cleanupPendingWindowsUpdate
	t.Cleanup(func() { cleanupPendingWindowsUpdate = original })

	called := false
	cleanupPendingWindowsUpdate = func() error {
		called = true
		return errors.New("expected cleanup failure")
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "version", "--json"}, strings.NewReader(""), &stdout, &stderr)

	if code != 1 || !called || !strings.Contains(stderr.String(), "clean pending update: expected cleanup failure") {
		t.Fatalf("normal startup cleanup contract failed: code=%d cleanup_called=%t stdout_bytes=%d stderr_bytes=%d", code, called, stdout.Len(), stderr.Len())
	}
}

func TestRunKnownFlagValueErrorSkipsStartupSideEffects(t *testing.T) {
	original := cleanupPendingWindowsUpdate
	t.Cleanup(func() { cleanupPendingWindowsUpdate = original })
	called := false
	cleanupPendingWindowsUpdate = func() error {
		called = true
		return nil
	}
	var stdout, stderr bytes.Buffer

	code := Run([]string{"pixiv", "--help=not-bool", "--help=false", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)

	if code != 1 || called || !strings.Contains(stderr.String(), "invalid argument") {
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

func TestRunUnknownOptionsAreNormalizedBeforeStartupSideEffects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	originalCleanup := cleanupPendingWindowsUpdate
	originalSupported := automaticPersistentHandlerSupported
	originalEnsure := ensureURLSchemeRelay
	t.Cleanup(func() {
		cleanupPendingWindowsUpdate = originalCleanup
		automaticPersistentHandlerSupported = originalSupported
		ensureURLSchemeRelay = originalEnsure
	})

	cleanupCalls := 0
	ensureCalls := 0
	mcpCalls := 0
	serviceCalls := 0
	cleanupPendingWindowsUpdate = func() error {
		cleanupCalls++
		return nil
	}
	automaticPersistentHandlerSupported = func() bool { return true }
	ensureURLSchemeRelay = func(context.Context) error {
		ensureCalls++
		return nil
	}
	oldMCP := runMCPServer
	runMCPServer = func(context.Context, *string, *time.Duration) error {
		mcpCalls++
		return nil
	}
	oldServices := newCLIServices
	newCLIServices = func() (*bootstrap.Runtime, error) {
		serviceCalls++
		return &bootstrap.Runtime{}, nil
	}
	t.Cleanup(func() {
		runMCPServer = oldMCP
		newCLIServices = oldServices
	})

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "root long", args: []string{"pixiv", "--unknown"}, want: "error: unknown option '--unknown'\n"},
		{name: "long", args: []string{"pixiv", "version", "--unknown"}, want: "error: unknown option '--unknown'\n"},
		{name: "long equals", args: []string{"pixiv", "version", "--unknown=value"}, want: "error: unknown option '--unknown'\n"},
		{name: "pixiv before positional", args: []string{"pixiv", "search", "--unknown", "miku"}, want: "error: unknown option '--unknown'\n"},
		{name: "pixiv after positional", args: []string{"pixiv", "search", "miku", "--unknown"}, want: "error: unknown option '--unknown'\n"},
		{name: "fanbox before positional", args: []string{"pixiv", "fanbox", "posts", "--unknown", "source"}, want: "error: unknown option '--unknown'\n"},
		{name: "pixiv mcp", args: []string{"pixiv", "mcp", "--unknown"}, want: "error: unknown option '--unknown'\n"},
		{name: "fanbox mcp", args: []string{"pixiv", "fanbox", "mcp", "--unknown"}, want: "error: unknown option '--unknown'\n"},
		{name: "combined short", args: []string{"pixiv", "version", "-xz"}, want: "error: unknown option '-x'\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, configPath := useTempPaths(t)
			cleanupCalls = 0
			ensureCalls = 0
			mcpCalls = 0
			serviceCalls = 0
			var stdout, stderr bytes.Buffer
			code := Run(test.args, strings.NewReader(""), &stdout, &stderr)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 || stderr.String() != test.want {
				t.Fatalf("output mismatch: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if cleanupCalls != 0 || ensureCalls != 0 || mcpCalls != 0 || serviceCalls != 0 {
				t.Fatalf("startup side effects ran after parse failure: cleanup=%d ensure=%d mcp=%d services=%d", cleanupCalls, ensureCalls, mcpCalls, serviceCalls)
			}
			if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("config initialization ran after parse failure: stat error=%v", err)
			}
		})
	}
}

func TestRunReportsNilRuntimeFactoryResult(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	oldServices := newCLIServices
	newCLIServices = func() (*bootstrap.Runtime, error) { return nil, nil }
	t.Cleanup(func() { newCLIServices = oldServices })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "miku"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run exit code = %d, want 1; stderr=%q", code, stderr.String())
	}
	if got := stderr.String(); got != "initialize local state: runtime factory returned nil\n" {
		t.Fatalf("stderr = %q, want explicit runtime initialization error", got)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runtime initialization failure wrote stdout: %q", stdout.String())
	}
}
