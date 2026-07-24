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

func TestInstallShSelectsFastestVerifiedSourceAndRetriesArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer is exercised by Unix platform jobs")
	}

	fixtureDir, assetName := prepareUnixFixture(t, false)
	fakeBin := prepareFakeCurl(t)
	logPath := filepath.Join(t.TempDir(), "curl.log")
	fastChecksumReady := filepath.Join(t.TempDir(), "fast-checksum-ready")
	fastChecksumGateArmed := filepath.Join(t.TempDir(), "fast-checksum-gate-armed")
	if output, err := exec.Command("mkfifo", fastChecksumReady).CombinedOutput(); err != nil {
		t.Fatalf("create fast checksum fixture gate: %v\n%s", err, output)
	}
	installDir := t.TempDir()
	command := exec.Command("sh", filepath.Join("..", "install.sh"), "--install-dir", installDir, "--no-path")
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PIXIV_INSTALLER_FIXTURES="+fixtureDir,
		"PIXIV_INSTALLER_CURL_LOG="+logPath,
		"PIXIV_INSTALLER_CURL_FAIL_ARCHIVE_HOST=fast.example",
		"PIXIV_INSTALLER_CURL_FAIL_CHECKSUM_HOST=fallback.example",
		// fast checksum 的成功写入会放行直连 checksum，避免并发 probe 的 scheduler
		// 偶然顺序掩盖“最快已验证候选被优先选中”的用户可见契约。
		"PIXIV_INSTALLER_CURL_WAIT_CHECKSUM_URL_PREFIX=https://github.com/",
		"PIXIV_INSTALLER_CURL_SIGNAL_CHECKSUM_URL_PREFIX=https://fast.example/",
		"PIXIV_INSTALLER_CURL_CHECKSUM_GATE="+fastChecksumReady,
		"PIXIV_INSTALLER_CURL_CHECKSUM_GATE_STATE="+fastChecksumGateArmed,
		"PIXIV_RELEASE_SOURCES=fast|-|https://fast.example/{url}\nfallback|-|https://fallback.example/{url}\ngithub-direct|{url}|{url}",
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

// TestInstallCmdCheckoutUsesCRLFWhenAutocrlfDisabled 固化 Windows 发布测试的
// checkout 契约：即使 Git for Windows 被要求关闭 autocrlf，cmd.exe 脚本也必须
// 由仓库属性以 CRLF 写入工作树，避免 cmd.exe 将 LF 脚本错误地连成一行。
func TestInstallCmdCheckoutUsesCRLFWhenAutocrlfDisabled(t *testing.T) {
	temporaryRepository := t.TempDir()
	for source, destination := range map[string]string{
		filepath.Join("..", "..", ".gitattributes"): filepath.Join(temporaryRepository, ".gitattributes"),
		filepath.Join("..", "install.cmd"):          filepath.Join(temporaryRepository, "scripts", "install.cmd"),
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

func TestInstallCmdBindsWindowsSystemTarForZIPExtraction(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "install.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	content := strings.ToLower(string(payload))
	for _, required := range []string{
		`set "system_tar=%systemroot%\system32\tar.exe"`,
		`if not exist "%system_tar%"`,
		`"%system_tar%" -xf "%asset%" -c "extract" pixiv.exe`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("install.cmd must bind Windows system tar for ZIP extraction; missing %q", required)
		}
	}
	if strings.Contains(content, `tar.exe -xf`) {
		t.Fatal("install.cmd must not resolve ZIP extraction through PATH")
	}
}

func TestInstallCmdExtractsArchiveFromTemporaryWorkingDirectory(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "install.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateInstallCmdExtraction(string(payload)); err != nil {
		t.Fatal(err)
	}
}

func TestInstallCmdExtractionContractRejectsMissingSuccessPopd(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "install.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	withoutSuccessPopd, ok := removeInstallCmdSuccessPopd(string(payload))
	if !ok {
		t.Fatal("test fixture did not locate the success-path popd")
	}
	if err := validateInstallCmdExtraction(withoutSuccessPopd); err == nil {
		t.Fatal("install.cmd extraction contract accepted a missing success-path popd")
	}
}

const installCmdSystemTarExtraction = `"%system_tar%" -xf "%asset%" -c "extract" pixiv.exe`

func removeInstallCmdSuccessPopd(script string) (string, bool) {
	normalized := strings.ReplaceAll(script, "\r\n", "\n")
	const successPath = "\npopd\nif not exist \"%EXTRACT_DIR%\\pixiv.exe\""
	withoutPopd := strings.Replace(normalized, successPath, strings.TrimPrefix(successPath, "\npopd"), 1)
	return withoutPopd, withoutPopd != normalized
}

func TestRemoveInstallCmdSuccessPopdHandlesCRLF(t *testing.T) {
	const script = "pushd \"%WORK_DIR%\"\r\n" +
		"\"%SYSTEM_TAR%\" -xf \"%ASSET%\" -C \"extract\" pixiv.exe || (popd & goto fatal)\r\n" +
		"popd\r\n" +
		"if not exist \"%EXTRACT_DIR%\\pixiv.exe\" goto fatal\r\n"

	mutated, ok := removeInstallCmdSuccessPopd(script)
	if !ok {
		t.Fatal("CRLF fixture did not locate the success-path popd")
	}
	if count := strings.Count(strings.ToLower(mutated), "popd"); count != 1 {
		t.Fatalf("mutation changed the failure-path popd: remaining count=%d, want 1", count)
	}
	if strings.Contains(strings.ReplaceAll(mutated, "\r\n", "\n"), "\npopd\n") {
		t.Fatal("mutation preserved the independent success-path popd")
	}
}

func validateInstallCmdExtraction(script string) error {
	content := strings.ToLower(strings.ReplaceAll(script, "\r\n", "\n"))
	pushIndex := strings.Index(content, `pushd "%work_dir%"`)
	tarIndex := strings.Index(content, installCmdSystemTarExtraction)
	if pushIndex < 0 || tarIndex < 0 || pushIndex > tarIndex {
		return fmt.Errorf("install.cmd must extract the relative archive inside its temporary working directory")
	}
	if strings.Contains(content, `-xf "%archive%"`) {
		return fmt.Errorf("install.cmd passed an absolute drive path directly to tar.exe")
	}

	lines := strings.Split(content, "\n")
	for index, line := range lines {
		if !strings.Contains(line, installCmdSystemTarExtraction) {
			continue
		}
		for next := index + 1; next < len(lines); next++ {
			trimmed := strings.TrimSpace(lines[next])
			if trimmed == "" || strings.HasPrefix(trimmed, "rem ") {
				continue
			}
			if trimmed != "popd" {
				return fmt.Errorf("install.cmd must restore the working directory after successful extraction")
			}
			return nil
		}
		return fmt.Errorf("install.cmd must restore the working directory after successful extraction")
	}
	return fmt.Errorf("install.cmd system tar invocation was not found")
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
case "${PIXIV_INSTALLER_CURL_WAIT_CHECKSUM_URL_PREFIX:-}" in
  '') ;;
  *)
    case "$url" in
      "$PIXIV_INSTALLER_CURL_WAIT_CHECKSUM_URL_PREFIX"*checksums.txt)
        # 首个直连请求是安装器的权威 checksum；随后才让并发的直连 probe
        # 等待 fast checksum 成功，以免其抢占 FIFO 结果。
        if [ -e "$PIXIV_INSTALLER_CURL_CHECKSUM_GATE_STATE" ]; then
          IFS= read -r _ < "$PIXIV_INSTALLER_CURL_CHECKSUM_GATE"
        fi
        : > "$PIXIV_INSTALLER_CURL_CHECKSUM_GATE_STATE"
        ;;
    esac
    ;;
esac
cp "$PIXIV_INSTALLER_FIXTURES/${url##*/}" "$output"
case "${PIXIV_INSTALLER_CURL_SIGNAL_CHECKSUM_URL_PREFIX:-}" in
  '') ;;
  *)
    case "$url" in
      "$PIXIV_INSTALLER_CURL_SIGNAL_CHECKSUM_URL_PREFIX"*checksums.txt)
        printf '%s\n' ready > "$PIXIV_INSTALLER_CURL_CHECKSUM_GATE"
        ;;
    esac
    ;;
esac
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
