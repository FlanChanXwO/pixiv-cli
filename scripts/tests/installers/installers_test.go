package installers_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	command := exec.Command("sh", filepath.Join("..", "..", "install.sh"), "--install-dir", installDir, "--no-path")
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_INSTALLER_FIXTURES="+fixtureDir,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("install verified fixture %s: %v\n%s", assetName, err, output)
	}

	installed := filepath.Join(installDir, "pixiv")
	result, err := exec.Command(installed, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("run installed fixture: %v\n%s", err, result)
	}
	if got, want := string(result), "pixiv v"+fixtureVersion+"\n"; got != want {
		t.Fatalf("installed fixture returned version %q, want %q", got, want)
	}
	if !strings.Contains(string(output), "SHA-256 verified") || !strings.Contains(string(output), installed) || !strings.Contains(string(output), "pixiv v"+fixtureVersion+"\n") {
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

	command := exec.Command("sh", filepath.Join("..", "..", "install.sh"), "--install-dir", installDir, "--no-path")
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

func TestInstallShVersionPreflightFailurePreservesExistingBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer is exercised by Unix platform jobs")
	}

	fixtureDir, _ := prepareUnixFixtureWithVersion(t, false, "0.0.0")
	fakeBin := prepareFakeCurl(t)
	installDir := t.TempDir()
	installed := filepath.Join(installDir, "pixiv")
	const sentinel = "existing-install-must-survive-version-preflight\n"
	if err := os.WriteFile(installed, []byte(sentinel), 0o755); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("sh", filepath.Join("..", "..", "install.sh"), "--install-dir", installDir, "--no-path")
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_INSTALLER_FIXTURES="+fixtureDir,
	)
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("version preflight mismatch unexpectedly succeeded:\n%s", output)
	}
	payload, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != sentinel {
		t.Fatalf("version preflight failure changed existing binary: %q", payload)
	}
}

func TestInstallShSelectsFastestVerifiedSourceAndRetriesArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer is exercised by Unix platform jobs")
	}

	fixtureDir, assetName := prepareUnixFixture(t, false)
	fakeBin := prepareFakeCurl(t)
	logPath := filepath.Join(t.TempDir(), "curl.log")
	installDir := t.TempDir()
	command := exec.Command("sh", filepath.Join("..", "..", "install.sh"), "--install-dir", installDir, "--no-path")
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_INSTALLER_FIXTURES="+fixtureDir,
		"PIXIV_INSTALLER_CURL_LOG="+logPath,
		"PIXIV_INSTALLER_CURL_FAIL_ARCHIVE_HOST=fast.example",
		"PIXIV_INSTALLER_CURL_FAIL_CHECKSUM_HOST=fallback.example",
		// 该 fixture 只比较两个受控公共候选，避免把并发 scheduler 与直连候选
		// 的完成顺序误当成“最快已验证公共源”的安装器行为。
		"PIXIV_RELEASE_SOURCES=fast|-|https://fast.example/{url}\nfallback|-|https://fallback.example/{url}",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("install with source retry: %v\n%s", err, output)
	}
	logBody, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBody)
	for _, required := range []string{
		"https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/checksums.txt",
		"https://fast.example/https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/checksums.txt",
		"https://fast.example/https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/" + assetName,
		"https://fallback.example/https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/" + assetName,
	} {
		if !strings.Contains(log, required) {
			t.Fatalf("curl log missing %q:\n%s", required, log)
		}
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
		command := exec.Command("sh", filepath.Join("..", "..", "install.sh"), "--add-to-path")
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
	command := exec.Command("sh", filepath.Join("..", "..", "install.sh"), "--install-dir", custom, "--add-to-path")
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

// TestInstallCmdCheckoutUsesCRLFWhenAutocrlfDisabled 固化 Windows 发布测试的
// checkout 契约：即使 Git for Windows 被要求关闭 autocrlf，cmd.exe 脚本也必须
// 由仓库属性以 CRLF 写入工作树，避免 cmd.exe 将 LF 脚本错误地连成一行。
func TestInstallCmdCheckoutUsesCRLFWhenAutocrlfDisabled(t *testing.T) {
	temporaryRepository := t.TempDir()
	for source, destination := range map[string]string{
		filepath.Join("..", "..", "..", ".gitattributes"): filepath.Join(temporaryRepository, ".gitattributes"),
		filepath.Join("..", "..", "install.cmd"):          filepath.Join(temporaryRepository, "scripts", "install.cmd"),
	} {
		payload, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	runGitCommand(t, temporaryRepository, "init", "--quiet")
	runGitCommand(t, temporaryRepository, "config", "user.email", "installer-test@example.invalid")
	runGitCommand(t, temporaryRepository, "config", "user.name", "installer test")
	runGitCommand(t, temporaryRepository, "add", ".gitattributes", "scripts/install.cmd")
	runGitCommand(t, temporaryRepository, "commit", "--quiet", "-m", "fixture")

	checkout := filepath.Join(t.TempDir(), "checkout")
	command := exec.Command("git", "-c", "core.autocrlf=false", "clone", "--quiet", temporaryRepository, checkout)
	command.Env = isolatedGitEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("clone fixture with autocrlf disabled: %v\n%s", err, output)
	}
	payload, err := os.ReadFile(filepath.Join(checkout, "scripts", "install.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	for lineNumber, line := range bytes.SplitAfter(payload, []byte("\n")) {
		if len(line) == 0 || line[len(line)-1] != '\n' {
			continue
		}
		if len(line) < 2 || line[len(line)-2] != '\r' {
			t.Fatalf("scripts/install.cmd checkout line %d uses LF instead of CRLF", lineNumber+1)
		}
	}
}

// installCmdInvocation 保持 Windows smoke 测试对 cmd.exe 参数边界的覆盖；
// 参数必须作为独立 argv 传入，不能预先拼接成带引号的命令行。
func installCmdInvocation(script, installDir string) []string {
	return []string{"/d", "/c", "call", script, "--install-dir", installDir, "--no-path"}
}

func runGitCommand(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	command.Env = isolatedGitEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
}

// isolatedGitEnvironment 移除提交 hook 注入的仓库位置变量。临时仓库命令若继承
// GIT_DIR 或 GIT_WORK_TREE，会把 fixture 的 init/config 写入正在提交的仓库。
func isolatedGitEnvironment() []string {
	locations := map[string]struct{}{
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": {},
		"GIT_COMMON_DIR":                   {},
		"GIT_DIR":                          {},
		"GIT_INDEX_FILE":                   {},
		"GIT_OBJECT_DIRECTORY":             {},
		"GIT_WORK_TREE":                    {},
	}
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, found := locations[name]; !found {
			environment = append(environment, entry)
		}
	}
	return environment
}

func prepareUnixFixture(t *testing.T, corruptChecksum bool) (string, string) {
	return prepareUnixFixtureWithVersion(t, corruptChecksum, fixtureVersion)
}

func prepareUnixFixtureWithVersion(t *testing.T, corruptChecksum bool, binaryVersion string) (string, string) {
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
		"pixiv": "#!/bin/sh\n[ \"${1:-}\" = --version ] || exit 64\nprintf 'pixiv v" + binaryVersion + "\\n'\n",
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
if [ -n "${PIXIV_INSTALLER_CURL_LOG:-}" ]; then
  printf '%s\n' "$url" >> "$PIXIV_INSTALLER_CURL_LOG"
fi
case "${PIXIV_INSTALLER_CURL_FAIL_ARCHIVE_HOST:-}" in
  '') ;;
  *)
    case "$url" in
      *"$PIXIV_INSTALLER_CURL_FAIL_ARCHIVE_HOST"*pixiv-cli_*.tar.gz) exit 22 ;;
    esac
    ;;
esac
case "${PIXIV_INSTALLER_CURL_FAIL_CHECKSUM_HOST:-}" in
  '') ;;
  *)
    case "$url" in
      *"$PIXIV_INSTALLER_CURL_FAIL_CHECKSUM_HOST"*/checksums.txt) exit 22 ;;
    esac
    ;;
esac
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
