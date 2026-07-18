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

	for name, args := range map[string][]string{
		"leading long":  {"pixiv", "--help=false", "auth", "export"},
		"leading short": {"pixiv", "-h=false", "auth", "export"},
		"between":       {"pixiv", "auth", "--help=false", "export"},
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			cleanupPendingWindowsUpdate = func() error {
				called = true
				return errors.New("cleanup must not run")
			}
			var stdout, stderr bytes.Buffer
			code := Run(args, strings.NewReader(""), &stdout, &stderr)

			if code != 0 || called || stdout.String() != "root-flag-export-secret\n" || stderr.Len() != 0 {
				t.Fatalf("root-flag export startup contract failed: code=%d cleanup_called=%t stdout_bytes=%d stderr_bytes=%d", code, called, stdout.Len(), stderr.Len())
			}
		})
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
