package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const versionSmokeConfig = "[update]\ncheck_enabled = false\n"

func TestPixivBinaryReportsBuildMetadata(t *testing.T) {
	repoRoot := ".."

	t.Run("development build uses defaults", func(t *testing.T) {
		assertPixivBuildMetadata(t, repoRoot, buildPixivVersionBinary(t, repoRoot), "dev", "unknown", "unknown")
	})

	t.Run("linker injected build uses release metadata", func(t *testing.T) {
		binaryPath := buildPixivVersionBinary(t, repoRoot,
			"-X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.Version=v0.1.0",
			"-X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.Commit=0123456789abcdef",
			"-X github.com/FlanChanXwO/pixiv-cli/internal/buildinfo.BuildDate=2026-07-11T00:00:00Z",
		)
		assertPixivBuildMetadata(t, repoRoot, binaryPath, "v0.1.0", "0123456789abcdef", "2026-07-11T00:00:00Z")
	})
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

	assertPixivBuildMetadata(t, "..", binaryPath, expectedVersion, "", "")
	for _, args := range [][]string{{"config", "path"}, {"mcp", "--help"}, {"fanbox", "mcp", "--help"}} {
		t.Run(fmt.Sprintf("%s %s", args[0], strings.Join(args[1:], " ")), func(t *testing.T) {
			stdout := runPixivVersionCommand(t, "..", binaryPath, args...)
			if strings.TrimSpace(stdout) == "" {
				t.Fatalf("pixiv %s returned empty stdout", strings.Join(args, " "))
			}
		})
	}
}

func assertPixivBuildMetadata(t *testing.T, repoRoot, binaryPath, version, commit, buildDate string) {
	t.Helper()

	t.Run("version text", func(t *testing.T) {
		stdout := runPixivVersionCommand(t, repoRoot, binaryPath, "version")
		if commit == "" || buildDate == "" {
			if !strings.HasPrefix(stdout, "pixiv "+version+"\ncommit: ") || !strings.Contains(stdout, "\nbuild date: ") {
				t.Fatalf("pixiv version stdout = %q, want version %q with build metadata", stdout, version)
			}
			return
		}
		want := "pixiv " + version + "\ncommit: " + commit + "\nbuild date: " + buildDate + "\n"
		if stdout != want {
			t.Fatalf("pixiv version stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("root version flag", func(t *testing.T) {
		stdout := runPixivVersionCommand(t, repoRoot, binaryPath, "--version")
		want := "pixiv " + version + "\n"
		if stdout != want {
			t.Fatalf("pixiv --version stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("version JSON", func(t *testing.T) {
		stdout := runPixivVersionCommand(t, repoRoot, binaryPath, "version", "--json")
		var metadata map[string]any
		if err := json.Unmarshal([]byte(stdout), &metadata); err != nil {
			t.Fatalf("pixiv version --json stdout is not a standalone JSON object: %v\n%s", err, stdout)
		}
		want := map[string]string{"version": version}
		if commit != "" || buildDate != "" {
			want["commit"] = commit
			want["build_date"] = buildDate
		}
		if commit != "" || buildDate != "" {
			if len(metadata) != len(want) {
				t.Fatalf("pixiv version --json fields = %#v, want exactly %#v", metadata, want)
			}
		} else if len(metadata) < len(want) {
			t.Fatalf("pixiv version --json fields = %#v, want at least %#v", metadata, want)
		}
		for field, wantValue := range want {
			gotValue, ok := metadata[field].(string)
			if !ok || gotValue != wantValue {
				t.Fatalf("pixiv version --json %q = %#v, want %q", field, metadata[field], wantValue)
			}
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
	t.Helper()

	run := exec.CommandContext(testCommandContext(t), binaryPath, args...)
	run.Dir = repoRoot
	run.Env = isolatedVersionProcessEnv(t)
	var stdout, stderr bytes.Buffer
	run.Stdout = &stdout
	run.Stderr = &stderr
	if err := run.Run(); err != nil {
		t.Fatalf("pixiv %s exited unsuccessfully: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("pixiv %s wrote to stderr:\n%s", strings.Join(args, " "), stderr.String())
	}
	return stdout.String()
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
