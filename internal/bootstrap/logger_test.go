package bootstrap

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewApplicationLoggerKeepsTerminalSilentWhenConfigIsMalformed(t *testing.T) {
	clearRuntimeEnvironment(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("LocalAppData", filepath.Join(home, "localapp"))
	configPath := filepath.Join(t.TempDir(), "config.toml")
	t.Cleanup(config.SetFilePathForTest(configPath))
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[logging\nlevel = broken")))
	var output bytes.Buffer
	global := slog.Default()

	logger, closer, err := NewApplicationLogger(&output)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closer.Close()) })
	logger.Error("must remain local")

	// 配置损坏时仍不得污染终端；文件日志尽力写入，失败则静默。
	require.Empty(t, output.String())
	require.Same(t, global, slog.Default())
}

func TestNewApplicationLoggerRejectsInvalidExplicitLoggingSettings(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		wantError string
	}{
		{name: "level", body: "[logging]\nlevel = \"verbose\"\nformat = \"text\"\n", wantError: "log_level must be one of"},
		{name: "format", body: "[logging]\nlevel = \"info\"\nformat = \"xml\"\n", wantError: "log_format must be text or json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearRuntimeEnvironment(t)
			configPath := filepath.Join(t.TempDir(), "config.toml")
			t.Cleanup(config.SetFilePathForTest(configPath))
			require.NoError(t, config.WritePrivateFile(configPath, []byte(test.body)))
			global := slog.Default()

			logger, closer, err := NewApplicationLogger(&bytes.Buffer{})

			require.Nil(t, logger)
			require.Nil(t, closer)
			require.ErrorContains(t, err, test.wantError)
			require.Same(t, global, slog.Default())
		})
	}
}

func TestNewApplicationLoggerKeepsTerminalSilentAndWritesJSONLFile(t *testing.T) {
	clearRuntimeEnvironment(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("LocalAppData", filepath.Join(home, "localapp"))

	configPath := filepath.Join(t.TempDir(), "config.toml")
	t.Cleanup(config.SetFilePathForTest(configPath))
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[logging]\nlevel = \"info\"\nformat = \"text\"\n")))

	var output bytes.Buffer
	global := slog.Default()
	logger, closer, err := NewApplicationLogger(&output)
	require.NoError(t, err)
	logger.Info("bootstrap logger probe", "component", "cli", "operation", "pixiv search")
	require.Same(t, global, slog.Default())
	// 终端不得出现日志痕迹
	require.Empty(t, output.String())

	logDir, err := DefaultLogDir()
	require.NoError(t, err)
	day := time.Now().Format("2006-01-02")
	path := filepath.Join(logDir, "pixiv-"+day+".jsonl")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(body), "bootstrap logger probe")
	require.NotContains(t, string(body), "refresh_token")
	require.True(t, strings.Contains(string(body), "operation"))
	// Windows 不允许删除仍被当前进程打开的文件。命令结束时必须主动关闭
	// writer，避免各次 CLI 调用遗留日志句柄。
	require.NoError(t, closer.Close())
	require.NoError(t, os.RemoveAll(home))
}
