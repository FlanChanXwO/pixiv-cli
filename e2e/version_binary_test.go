package e2e

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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

func assertPixivBuildMetadata(t *testing.T, repoRoot, binaryPath, version, commit, buildDate string) {
	t.Helper()

	t.Run("version text", func(t *testing.T) {
		stdout := runPixivVersionCommand(t, repoRoot, binaryPath, "version")
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
		want := map[string]string{
			"version":    version,
			"commit":     commit,
			"build_date": buildDate,
		}
		if len(metadata) != len(want) {
			t.Fatalf("pixiv version --json fields = %#v, want exactly %#v", metadata, want)
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
		if found && isWindowsProfileEnvironmentVariable(name) {
			continue
		}
		values = append(values, entry)
	}
	return append(values,
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
