package main_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const versionSmokeConfig = "[update]\ncheck_enabled = false\n"

// TestPixivBinaryPackagedSmoke 验证平台 workflow 解包后的实际二进制，而不是重新构建一个开发二进制。
// 该测试只在 workflow 注入路径与期望版本时运行；不读取凭据，也不启用真实 SDK E2E。
func TestPixivBinaryPackagedSmoke(t *testing.T) {
	binaryPath := strings.TrimSpace(os.Getenv("PIXIV_E2E_BINARY"))
	if binaryPath == "" {
		t.Skip("PIXIV_E2E_BINARY is not set")
	}
	if !filepath.IsAbs(binaryPath) {
		t.Fatal("PIXIV_E2E_BINARY must be an absolute path")
	}
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("stat packaged binary: %v", err)
	}
	expectedVersion := strings.TrimSpace(os.Getenv("PIXIV_E2E_EXPECTED_VERSION"))
	if expectedVersion == "" {
		t.Fatal("PIXIV_E2E_EXPECTED_VERSION is required with PIXIV_E2E_BINARY")
	}

	assertPixivVersionContract(t, binaryPath, expectedVersion)
	for _, args := range [][]string{{"config", "path"}, {"mcp", "--help"}, {"fanbox", "--help"}} {
		t.Run(fmt.Sprintf("%s %s", args[0], strings.Join(args[1:], " ")), func(t *testing.T) {
			stdout := runPixivCommand(t, binaryPath, args...)
			if strings.TrimSpace(stdout) == "" {
				t.Fatalf("pixiv %s returned empty stdout", strings.Join(args, " "))
			}
		})
	}
	t.Run("removed FANBOX MCP command", func(t *testing.T) {
		stdout, stderr, err := runPixivProcess(t, binaryPath, "fanbox", "mcp")
		if err == nil || stdout != "" || !strings.Contains(stderr, "usage: pixiv fanbox <command>") {
			t.Fatalf("removed FANBOX MCP command: err=%v stdout=%q stderr=%q", err, stdout, stderr)
		}
	})
}

const repoRoot = "../.."

func assertPixivVersionContract(t *testing.T, binaryPath, version string) {
	t.Helper()
	stdout := runPixivCommand(t, binaryPath, "--version")
	if want := "pixiv " + version + "\n"; stdout != want {
		t.Fatalf("pixiv --version stdout = %q, want %q", stdout, want)
	}
}

func runPixivCommand(t *testing.T, binaryPath string, args ...string) string {
	t.Helper()
	stdout, stderr, err := runPixivProcess(t, binaryPath, args...)
	if err != nil {
		t.Fatalf("pixiv %s exited unsuccessfully: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("pixiv %s wrote to stderr:\n%s", strings.Join(args, " "), stderr)
	}
	return stdout
}

func runPixivProcess(t *testing.T, binaryPath string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	run := exec.CommandContext(t.Context(), binaryPath, args...)
	run.Dir = repoRoot
	run.Env = isolatedEnv(t).values
	var stdoutBuffer, stderrBuffer bytes.Buffer
	run.Stdout = &stdoutBuffer
	run.Stderr = &stderrBuffer
	err = run.Run()
	return stdoutBuffer.String(), stderrBuffer.String(), err
}

// isolatedProcessEnv 为子进程提供与宿主用户目录和 CLI 覆写隔离的环境。
type isolatedProcessEnv struct {
	values []string
	home   string
}

// isolatedEnv 同时关闭临时用户目录中的自动更新，避免网络提示污染命令 stderr。
func isolatedEnv(t *testing.T) isolatedProcessEnv {
	var env isolatedProcessEnv
	t.Helper()
	env.home = t.TempDir()
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if found && isIsolatedEnvKey(name) {
			continue
		}
		env.values = append(env.values, entry)
	}
	env.values = append(env.values,
		"HOME="+env.home,
		"APPDATA="+env.home,
		"LOCALAPPDATA="+env.home,
		"USERPROFILE="+env.home,
	)
	configDirectory := filepath.Join(env.home, ".pixiv-cli")
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		t.Fatalf("create isolated config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDirectory, "config.toml"), []byte(versionSmokeConfig), 0o600); err != nil {
		t.Fatalf("write isolated config: %v", err)
	}
	return env
}

func isIsolatedEnvKey(name string) bool {
	for _, key := range []string{
		"HOME", "XDG_CONFIG_HOME", "APPDATA", "LOCALAPPDATA", "USERPROFILE", "HOMEDRIVE", "HOMEPATH",
		"DOWNLOAD_PATH", "FILENAME_TEMPLATE", "https_proxy", "HTTPS_PROXY",
	} {
		if strings.EqualFold(name, key) {
			return true
		}
	}
	return false
}
