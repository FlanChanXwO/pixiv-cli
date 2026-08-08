package sqliteio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/core"
)

func TestQueryMapsPermissionFailureWithoutLeakingCommandOutput(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "sqlite3")
	contents := "#!/bin/sh\nprintf '%s\\n' 'permission denied: fixture-secret' >&2\nexit 1\n"
	if runtime.GOOS == "windows" {
		command += ".cmd"
		contents = "@echo permission denied: fixture-secret 1>&2\r\n@exit /b 1\r\n"
	}
	if err := os.WriteFile(command, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)

	_, err := Query(context.Background(), filepath.Join(dir, "Cookies"), "SELECT 1", nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Fatalf("err = %v, want ErrPermissionDenied", err)
	}
	if err.Error() == "permission denied: fixture-secret" {
		t.Fatal("sqlite3 stderr leaked into error")
	}
}
