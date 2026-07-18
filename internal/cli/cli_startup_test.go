package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
)

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
		{name: "version one", args: []string{"pixiv", "--version=1", "auth", "export"}},
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

	code := Run([]string{"pixiv", "--help=not-bool", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)

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
