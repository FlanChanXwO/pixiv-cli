package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const versionSmokeConfig = "[update]\ncheck_enabled = false\n"

func TestPixivBinaryReportsRootVersion(t *testing.T) {
	repoRoot := ".."

	t.Run("development build uses defaults", func(t *testing.T) {
		assertPixivVersionContract(t, repoRoot, buildPixivVersionBinary(t, repoRoot), "dev")
	})

	t.Run("linker injected build uses release version", func(t *testing.T) {
		binaryPath := buildPixivVersionBinary(t, repoRoot,
			"-X github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo.Version=v0.1.0",
		)
		assertPixivVersionContract(t, repoRoot, binaryPath, "v0.1.0")
	})
}

func TestPixivBinaryRejectsRemovedVersionCommand(t *testing.T) {
	binaryPath := buildPixivVersionBinary(t, "..")
	for _, args := range [][]string{{"version"}, {"version", "--json"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, err := runPixivVersionProcess(t, "..", binaryPath, args...)
			if err == nil {
				t.Fatalf("pixiv %s succeeded, want unknown command", strings.Join(args, " "))
			}
			if stdout != "" {
				t.Fatalf("pixiv %s stdout = %q, want empty", strings.Join(args, " "), stdout)
			}
			if !strings.Contains(stderr, `unknown command "version"`) {
				t.Fatalf("pixiv %s stderr = %q, want unknown command", strings.Join(args, " "), stderr)
			}
		})
	}
}

func TestIsolatedVersionProcessEnvDisablesAutomaticUpdate(t *testing.T) {
	values := isolatedVersionProcessEnv(t)
	environment := make(map[string]string, len(values))
	for _, entry := range values {
		name, value, found := strings.Cut(entry, "=")
		if found {
			environment[name] = value
		}
	}
	home := environment["HOME"]
	if home == "" {
		t.Fatal("isolated version environment must define HOME")
	}
	body, err := os.ReadFile(filepath.Join(home, ".pixiv-cli", "config.toml"))
	if err != nil {
		t.Fatalf("read isolated version config: %v", err)
	}
	if string(body) != versionSmokeConfig {
		t.Fatalf("isolated version config = %q, want automatic update disabled", body)
	}
}

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

	assertPixivVersionContract(t, "..", binaryPath, expectedVersion)
	for _, args := range [][]string{{"config", "path"}, {"mcp", "--help"}, {"fanbox", "mcp", "--help"}} {
		t.Run(fmt.Sprintf("%s %s", args[0], strings.Join(args[1:], " ")), func(t *testing.T) {
			stdout := runPixivVersionCommand(t, "..", binaryPath, args...)
			if strings.TrimSpace(stdout) == "" {
				t.Fatalf("pixiv %s returned empty stdout", strings.Join(args, " "))
			}
		})
	}
}

func assertPixivVersionContract(t *testing.T, repoRoot, binaryPath, version string) {
	t.Helper()

	t.Run("root version flag", func(t *testing.T) {
		stdout := runPixivVersionCommand(t, repoRoot, binaryPath, "--version")
		want := "pixiv " + version + "\n"
		if stdout != want {
			t.Fatalf("pixiv --version stdout = %q, want %q", stdout, want)
		}
	})

}

func buildPixivVersionBinary(t *testing.T, repoRoot string, ldflags ...string) string {
	t.Helper()

	binaryName := "pixiv"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	args := []string{"build", "-trimpath", "-o", binaryPath}
	if len(ldflags) > 0 {
		args = append(args, "-ldflags", strings.Join(ldflags, " "))
	}
	args = append(args, "./cmd/pixiv")

	build := exec.CommandContext(testCommandContext(t), "go", args...)
	build.Dir = repoRoot
	var stderr bytes.Buffer
	build.Stderr = &stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return binaryPath
}

func runPixivVersionCommand(t *testing.T, repoRoot, binaryPath string, args ...string) string {
	stdout, stderr, err := runPixivVersionProcess(t, repoRoot, binaryPath, args...)
	if err != nil {
		t.Fatalf("pixiv %s exited unsuccessfully: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("pixiv %s wrote to stderr:\n%s", strings.Join(args, " "), stderr)
	}
	return stdout
}

func runPixivVersionProcess(t *testing.T, repoRoot, binaryPath string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	run := exec.CommandContext(testCommandContext(t), binaryPath, args...)
	run.Dir = repoRoot
	run.Env = isolatedVersionProcessEnv(t)
	var stdoutBuffer, stderrBuffer bytes.Buffer
	run.Stdout = &stdoutBuffer
	run.Stderr = &stderrBuffer
	err = run.Run()
	return stdoutBuffer.String(), stderrBuffer.String(), err
}

func isolatedVersionProcessEnv(t *testing.T) []string {
	t.Helper()

	profileRoot := t.TempDir()
	base := isolatedEnv(t).values
	values := make([]string, 0, len(base)+3)
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if found && (strings.EqualFold(name, "HOME") || isWindowsProfileEnvironmentVariable(name)) {
			continue
		}
		values = append(values, entry)
	}
	// 打包 smoke 使用带发布版本号的 CI 构建；自动更新检查不属于该二进制/归档
	// 契约，且其网络提示会污染命令 stderr。仅在临时用户目录关闭它，不改默认配置。
	configDirectory := filepath.Join(profileRoot, ".pixiv-cli")
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		t.Fatalf("create isolated version config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDirectory, "config.toml"), []byte(versionSmokeConfig), 0o600); err != nil {
		t.Fatalf("write isolated version config: %v", err)
	}
	return append(values,
		"HOME="+profileRoot,
		"APPDATA="+profileRoot,
		"LOCALAPPDATA="+profileRoot,
		"USERPROFILE="+profileRoot,
	)
}

func isWindowsProfileEnvironmentVariable(name string) bool {
	switch strings.ToUpper(name) {
	case "APPDATA", "LOCALAPPDATA", "USERPROFILE":
		return true
	default:
		return false
	}
}
