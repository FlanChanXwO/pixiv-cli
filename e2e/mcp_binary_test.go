package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPixivBinaryMCPCancelsOnSignals 验证真实入口把进程信号传入已有 HTTP 关闭路径；
// 在途工具的取消与清理顺序由 mcpserver 的 HTTP lifecycle 测试覆盖。
func TestPixivBinaryMCPCancelsOnSignals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal cannot deliver interrupt/termination signals on Windows")
	}
	binaryPath := strings.TrimSpace(os.Getenv("PIXIV_E2E_BINARY"))
	if binaryPath == "" {
		binaryPath = buildPixivVersionBinary(t, "..")
	}
	require.True(t, filepath.IsAbs(binaryPath), "binary path must be absolute")
	binaryPath, err := filepath.EvalSymlinks(binaryPath)
	require.NoError(t, err)
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			env := isolatedEnv(t)
			appDir := filepath.Join(env.home, ".pixiv-cli")
			require.NoError(t, os.MkdirAll(filepath.Join(appDir, "url-handler"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(appDir, "config.toml"), []byte(versionSmokeConfig), 0o600))
			// 临时 manifest 只避免启动钩子修改真实 OS 协议关联，不代表 helper 已安装。
			manifest, err := json.Marshal(map[string]any{"version": 1, "executable_path": binaryPath, "home_directory": env.home})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(appDir, "url-handler", "handler-manifest.json"), manifest, 0o600))

			ctx, cancel := context.WithCancel(testCommandContext(t))
			defer cancel()
			init := exec.CommandContext(ctx, binaryPath, "mcp", "auth", "init")
			init.Dir, init.Env = "..", env.values
			// owner secret 的一次性输出丢弃；本测试只需要服务启动，不进行授权。
			require.NoError(t, init.Run())
			reservation, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			addr := reservation.Addr().String()
			require.NoError(t, reservation.Close())
			endpoint := "http://" + addr + "/mcp"
			command := exec.CommandContext(ctx, binaryPath, "mcp", "--listen-addr", addr, "--base-url", "http://"+addr)
			command.Dir, command.Env = "..", env.values
			var stdout bytes.Buffer
			command.Stdout = &stdout
			stderr, err := command.StderrPipe()
			require.NoError(t, err)
			require.NoError(t, command.Start())
			waited := false
			defer func() {
				cancel()
				if !waited {
					_ = command.Wait()
				}
			}()
			reader := bufio.NewReader(stderr)
			line, err := reader.ReadString('\n')
			require.NoError(t, err)
			require.Contains(t, line, "MCP owner initialized; endpoint: "+endpoint)
			transport := &http.Transport{}
			defer transport.CloseIdleConnections()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			require.NoError(t, err)
			response, err := (&http.Client{Transport: transport}).Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusUnauthorized, response.StatusCode)

			require.NoError(t, command.Process.Signal(signal))
			tail, err := io.ReadAll(reader)
			require.NoError(t, err)
			err = command.Wait()
			waited = true
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, "signal must report CLI cancellation")
			require.Equal(t, 1, exit.ExitCode(), "signal must reach CLI cleanup, not terminate the process directly")
			require.Contains(t, string(tail), "context canceled")
			require.Empty(t, stdout.String(), "MCP must not write JSON-RPC or secrets to stdout")
		})
	}
}
