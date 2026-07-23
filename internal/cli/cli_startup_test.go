package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
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

func TestAuthURLCallbackRejectsInputWithoutEchoingIt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	const callback = "https://example.invalid/callback?code=one-time-code"
	code := Run([]string{"pixiv", "auth", internalURLCallbackCommand, callback}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "invalid Pixiv callback URL") || strings.Contains(stderr.String(), callback) || stdout.Len() != 0 {
		t.Fatalf("internal URL callback error contract failed: code=%d stdout_bytes=%d stderr=%q", code, stdout.Len(), stderr.String())
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
