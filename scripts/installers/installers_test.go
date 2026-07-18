package installers_test

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const fixtureVersion = "9.8.7"

func TestInstallShInstallsVerifiedLatestArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer is exercised by Unix platform jobs")
	}

	fixtureDir, assetName := prepareUnixFixture(t, false)
	fakeBin := prepareFakeCurl(t)
	installDir := filepath.Join(t.TempDir(), "install dir with spaces")

	command := exec.Command("sh", filepath.Join("..", "install.sh"), "--install-dir", installDir, "--no-path")
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_INSTALLER_FIXTURES="+fixtureDir,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("install verified fixture %s: %v\n%s", assetName, err, output)
	}

	installed := filepath.Join(installDir, "pixiv")
	result, err := exec.Command(installed, "version", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("run installed fixture: %v\n%s", err, result)
	}
	if !strings.Contains(string(result), `"version":"v`+fixtureVersion+`"`) {
		t.Fatalf("installed fixture returned unexpected version: %s", result)
	}
	if !strings.Contains(string(output), "SHA-256 verified") || !strings.Contains(string(output), installed) {
		t.Fatalf("installer did not report verification and destination:\n%s", output)
	}
}

func TestInstallShChecksumFailurePreservesExistingBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer is exercised by Unix platform jobs")
	}

	fixtureDir, _ := prepareUnixFixture(t, true)
	fakeBin := prepareFakeCurl(t)
	installDir := t.TempDir()
	installed := filepath.Join(installDir, "pixiv")
	const sentinel = "existing-install-must-survive\n"
	if err := os.WriteFile(installed, []byte(sentinel), 0o755); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("sh", filepath.Join("..", "install.sh"), "--install-dir", installDir, "--no-path")
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_INSTALLER_FIXTURES="+fixtureDir,
	)
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("checksum mismatch unexpectedly succeeded:\n%s", output)
	}
	payload, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != sentinel {
		t.Fatalf("checksum failure changed existing binary: %q", payload)
	}
}

func TestInstallShAddsDefaultDirectoryToPathIdempotently(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer is exercised by Unix platform jobs")
	}

	fixtureDir, _ := prepareUnixFixture(t, false)
	fakeBin := prepareFakeCurl(t)
	home := t.TempDir()
	environment := append(os.Environ(),
		"HOME="+home,
		"SHELL=/bin/bash",
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_INSTALLER_FIXTURES="+fixtureDir,
	)
	for iteration := 0; iteration < 2; iteration++ {
		command := exec.Command("sh", filepath.Join("..", "install.sh"), "--add-to-path")
		command.Env = environment
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("install with PATH update iteration %d: %v\n%s", iteration, err, output)
		}
	}

	profile, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatal(err)
	}
	const pathLine = `export PATH="$HOME/.local/bin:$PATH"`
	if count := strings.Count(string(profile), pathLine); count != 1 {
		t.Fatalf("PATH line count = %d, want 1:\n%s", count, profile)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "pixiv")); err != nil {
		t.Fatalf("default install missing: %v", err)
	}
}

func TestInstallShRejectsAutomaticPathForCustomDirectoryBeforeDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer is exercised by Unix platform jobs")
	}

	home := t.TempDir()
	custom := filepath.Join(t.TempDir(), "custom")
	command := exec.Command("sh", filepath.Join("..", "install.sh"), "--install-dir", custom, "--add-to-path")
	command.Env = append(os.Environ(), "HOME="+home)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("custom automatic PATH unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "only supports the default") {
		t.Fatalf("unexpected custom PATH error:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(custom, "pixiv")); !os.IsNotExist(err) {
		t.Fatalf("custom PATH failure reached installation: %v", err)
	}
}

func TestReadmesExposeBothInstallersAndAgentPrompt(t *testing.T) {
	for _, candidate := range []string{
		filepath.Join("..", "..", "README.md"),
		filepath.Join("..", "..", "README.zh-CN.md"),
		filepath.Join("..", "..", "README.ja.md"),
	} {
		payload, err := os.ReadFile(candidate)
		if err != nil {
			t.Fatal(err)
		}
		content := string(payload)
		for _, required := range []string{"scripts/install.sh", "scripts/install.cmd", "SHA-256", "pixiv version"} {
			if !strings.Contains(content, required) {
				t.Errorf("%s missing %q", candidate, required)
			}
		}
	}
}

func TestInstallCmdUsesOnlyNativeCmdTools(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "install.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(payload))
	for _, required := range []string{"curl.exe", "certutil.exe", "tar.exe", "reg.exe", "--install-dir", "--no-path"} {
		if !strings.Contains(lower, required) {
			t.Errorf("install.cmd missing %q", required)
		}
	}
	for _, forbidden := range []string{"powershell", ".ps1", "executionpolicy"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("install.cmd must not depend on %q", forbidden)
		}
	}
}

func TestInstallCmdInvocationKeepsPathsAsSeparateArguments(t *testing.T) {
	script := `C:\workspace with spaces\scripts\install.cmd`
	installDir := `C:\Users\tester\pixiv bin`
	want := []string{"/d", "/c", "call", script, "--install-dir", installDir, "--no-path"}
	if got := installCmdInvocation(script, installDir); !slices.Equal(got, want) {
		t.Fatalf("install.cmd invocation arguments mismatch: got %#v; want %#v", got, want)
	}
}

// installCmdInvocation 让 Go 分别引用每个 Windows 参数；若先拼接含引号的
// command line，os/exec 会为 cmd.exe 再次转义，脚本名可能变成字面 \"path\"。
func installCmdInvocation(script, installDir string) []string {
	return []string{"/d", "/c", "call", script, "--install-dir", installDir, "--no-path"}
}

func prepareUnixFixture(t *testing.T, corruptChecksum bool) (string, string) {
	t.Helper()
	targetOS := runtime.GOOS
	if targetOS != "linux" && targetOS != "darwin" {
		t.Fatalf("unsupported Unix test OS %q", targetOS)
	}
	targetArch := map[string]string{"amd64": "amd64", "arm64": "arm64"}[runtime.GOARCH]
	if targetArch == "" {
		t.Fatalf("unsupported Unix test architecture %q", runtime.GOARCH)
	}

	directory := t.TempDir()
	assetName := fmt.Sprintf("pixiv-cli_%s_%s_%s.tar.gz", fixtureVersion, targetOS, targetArch)
	assetPath := filepath.Join(directory, assetName)
	writeTarGz(t, assetPath, map[string]string{
		"pixiv": "#!/bin/sh\n[ \"${1:-}\" = version ] || exit 64\nprintf '{\"version\":\"v" + fixtureVersion + "\"}\\n'\n",
	})
	payload, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	if corruptChecksum {
		digest = strings.Repeat("0", sha256.Size*2)
	}
	checksums := digest + "  " + assetName + "\n" +
		strings.Repeat("1", sha256.Size*2) + "  install.cmd\n" +
		strings.Repeat("2", sha256.Size*2) + "  install.sh\n"
	if err := os.WriteFile(filepath.Join(directory, "checksums.txt"), []byte(checksums), 0o600); err != nil {
		t.Fatal(err)
	}
	return directory, assetName
}

func prepareFakeCurl(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	script := `#!/bin/sh
set -eu
output=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o|--output) output=$2; shift 2 ;;
    -*) shift ;;
    *) url=$1; shift ;;
  esac
done
[ -n "$output" ] && [ -n "$url" ]
cp "$PIXIV_INSTALLER_FIXTURES/${url##*/}" "$output"
`
	path := filepath.Join(directory, "curl")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return directory
}

func writeTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, body := range files {
		header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tarWriter, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
