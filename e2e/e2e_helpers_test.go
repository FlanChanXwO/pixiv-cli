package e2e

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// isolatedProcessEnv 为子进程提供与宿主用户目录和 CLI 覆写隔离的环境。
type isolatedProcessEnv struct {
	values []string
	home   string
}

func isolatedEnv(t *testing.T) isolatedProcessEnv {
	t.Helper()

	home := t.TempDir()
	filtered := make([]string, 0, len(os.Environ())+7)
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if found && isIsolatedEnvKey(name) {
			continue
		}
		filtered = append(filtered, entry)
	}
	filtered = append(filtered, "HOME="+home)
	if runtime.GOOS == "windows" {
		// Windows 的 os.UserHomeDir 优先读取 USERPROFILE，不能继承 runner 的用户目录。
		volume := filepath.VolumeName(home)
		filtered = append(filtered,
			"USERPROFILE="+home,
			"HOMEDRIVE="+volume,
			"HOMEPATH="+strings.TrimPrefix(home, volume),
		)
	}
	return isolatedProcessEnv{values: filtered, home: home}
}

func isIsolatedEnvKey(name string) bool {
	for _, key := range []string{
		"HOME", "XDG_CONFIG_HOME", "APPDATA", "LOCALAPPDATA", "USERPROFILE", "HOMEDRIVE", "HOMEPATH",
		"DOWNLOAD_PATH", "FILENAME_TEMPLATE", "https_proxy", "HTTPS_PROXY", "PIXIV_REFRESH_TOKEN",
	} {
		if strings.EqualFold(name, key) {
			return true
		}
	}
	return false
}

func testCommandContext(t *testing.T) context.Context {
	t.Helper()

	if deadline, ok := t.Deadline(); ok {
		ctx, cancel := context.WithDeadline(context.Background(), deadline.Add(-time.Second))
		t.Cleanup(cancel)
		return ctx
	}
	return context.Background()
}
