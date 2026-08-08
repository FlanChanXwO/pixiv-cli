package auth

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
	"github.com/stretchr/testify/require"
)

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
	base := filepath.Join(home, filesystem.AppDataDirName)
	configPath := filepath.Join(base, "config.toml")
	t.Cleanup(configapp.SetFilePathForTest(configPath))
	return authdb.DatabasePath(base), configPath
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
