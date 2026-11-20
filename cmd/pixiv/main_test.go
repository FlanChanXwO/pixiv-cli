package main_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli"
)

func TestInterfaceLanguage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	for _, command := range []string{"", "fanbox", "vector"} {
		args := []string{"pixiv"}
		if command != "" {
			args = append(args, command)
		}
		args = append(args, "--help")
		var out, diagnostic bytes.Buffer
		if code := cli.Run(context.Background(), args, strings.NewReader(""), &out, &diagnostic); code != 0 || diagnostic.Len() != 0 {
			t.Fatalf("%v: code=%d stderr=%s", args, code, &diagnostic)
		}
		if !strings.Contains(out.String(), "用法：") || !strings.Contains(out.String(), "选项：") {
			t.Fatalf("%v: untranslated help: %s", args, &out)
		}
	}
	var out, diagnostic bytes.Buffer
	if code := cli.Run(context.Background(), []string{"pixiv", "--unknown"}, strings.NewReader(""), &out, &diagnostic); code != 2 || !strings.Contains(diagnostic.String(), "未知选项") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &diagnostic)
	}
}

func TestInterfaceLanguageKeepsMachineOutput(t *testing.T) {
	home := isolatedHome(t)
	seedPixivAccount(t, home, "Bookmark · 原文用户", true)
	t.Setenv("LC_ALL", "C")
	english, diagnostic, code := runCLI(t, "whoami", "--json")
	if code != 0 || diagnostic != "" {
		t.Fatalf("English JSON: code=%d stderr=%s", code, diagnostic)
	}
	t.Setenv("LC_ALL", "zh_TW.UTF-8")
	chinese, diagnostic, code := runCLI(t, "whoami", "--json")
	if code != 0 || diagnostic != "" || chinese != english {
		t.Fatalf("Chinese JSON changed: code=%d stdout=%s stderr=%s", code, chinese, diagnostic)
	}
	human, diagnostic, code := runCLI(t, "whoami")
	if code != 0 || diagnostic != "" || human != "Bookmark · 原文用户 (12345678) · 高级会员\n" {
		t.Fatalf("Chinese text: code=%d stdout=%s stderr=%s", code, human, diagnostic)
	}
}

func TestInterfaceInputDiagnostics(t *testing.T) {
	isolatedHome(t)
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"does-not-exist"}, `未知命令 "does-not-exist"（命令路径："pixiv"）`},
		{[]string{"rankign"}, `你可能想输入：\n\tranking`},
		{[]string{"search", "cat", "--limit", "-1"}, "limit 必须为零或正整数"},
		{[]string{"search", "cat", "--page", "0"}, "page 必须为正整数"},
		{[]string{"search", "cat", "--page", "2"}, "--page 需要正整数 --limit"},
		{[]string{"search", "cat", "--limit", fmt.Sprint(int(^uint(0) >> 1)), "--page", "3"}, "page 和 limit 导致逻辑结果偏移量溢出"},
		{[]string{"fanbox", "posts", "artist", "--limit", "-1"}, "limit 必须为零或正整数"},
	} {
		stdout, stderr, code := runCLI(t, tc.args...)
		if code != 1 || stdout != "" || !strings.Contains(stderr, tc.want) {
			t.Fatalf("%v: code=%d stdout=%s stderr=%s", tc.args, code, stdout, stderr)
		}
	}
}

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

// isolatedEnv 固定英文并关闭临时目录中的自动更新，避免宿主语言和网络提示影响断言。
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
		"LC_ALL=C",
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
		"LC_ALL", "LC_MESSAGES", "LANG",
		"HOME", "XDG_CONFIG_HOME", "APPDATA", "LOCALAPPDATA", "USERPROFILE", "HOMEDRIVE", "HOMEPATH",
		"DOWNLOAD_PATH", "FILENAME_TEMPLATE", "https_proxy", "HTTPS_PROXY",
	} {
		if strings.EqualFold(name, key) {
			return true
		}
	}
	return false
}
