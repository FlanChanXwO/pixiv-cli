// Command browsernativeevidence 校验无 credential 的 browser provider workflow。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	checkoutAction = "actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8"
	setupGoAction  = "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff"
)

func main() {
	switch {
	case len(os.Args) == 4 && os.Args[1] == "policy" && os.Args[2] == "--workflow":
		if err := Validate(os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "browser evidence: %v\n", err)
			os.Exit(1)
		}
	case len(os.Args) == 4 && os.Args[1] == "firefox-contract" && os.Args[2] == "--firefox":
		if err := runFirefoxContract(os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "browser evidence: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: browsernativeevidence policy --workflow PATH | firefox-contract --firefox PATH")
		os.Exit(2)
	}
}

// Validate 保持 CI 入口小而无 credential。workflow 只有固定的受审计命令面，
// 因此用文本 policy 检查未知 action 引用和 secret-shaped 输入并 fail closed。
func Validate(path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read workflow: %w", err)
	}
	workflow := string(body)
	if err := validateWorkflowMatrices(body); err != nil {
		return err
	}
	for _, required := range []string{
		"name: Browser provider native contracts",
		"workflow_dispatch: {}",
		"permissions: {}",
		"browser_provider:",
		"runs-on: ${{ matrix.runner }}",
		"fail-fast: false",
		"runner: macos-15-intel\n            goos: darwin\n            goarch: amd64",
		"runner: macos-15\n            goos: darwin\n            goarch: arm64",
		"runner: ubuntu-22.04\n            goos: linux\n            goarch: amd64",
		"runner: ubuntu-22.04-arm\n            goos: linux\n            goarch: arm64",
		"runner: windows-2025\n            goos: windows\n            goarch: amd64",
		"runner: windows-11-arm\n            goos: windows\n            goarch: arm64",
		"firefox_native:",
		"go run ./scripts/browsernativeevidence firefox-contract --firefox",
		"Firefox version: 153.0.3",
		"Firefox package SHA-256:",
		"Firefox runner target:",
		"firefox_url: https://ftp.mozilla.org/pub/firefox/releases/153.0.3/linux-aarch64/en-US/firefox-153.0.3.tar.xz",
		"firefox_sha256: c19b325accedebbc3a1235e3c7104d80c5a4412b368f7d0935b4718114416870",
		"firefox_url: https://ftp.mozilla.org/pub/firefox/releases/153.0.3/linux-x86_64/en-US/firefox-153.0.3.tar.xz",
		"firefox_sha256: 22b312280900bfb174b685ece32c7b3c6d72e7f8e53d6d30f21ac41a8dc500a2",
		"firefox_url: https://ftp.mozilla.org/pub/firefox/releases/153.0.3/mac/en-US/Firefox%20153.0.3.dmg",
		"firefox_sha256: a0523b6f2f10f13c6071d8b53ed7678193d693febd8a5d4fd8d7417b3c661045",
		"firefox_url: https://ftp.mozilla.org/pub/firefox/releases/153.0.3/win64/en-US/Firefox%20Setup%20153.0.3.exe",
		"firefox_sha256: 8de41917930c35937a46eac6d0e16c633ed7456c771b32b89dc6fd65d55e512e",
		"firefox_url: https://ftp.mozilla.org/pub/firefox/releases/153.0.3/win64-aarch64/en-US/Firefox%20Setup%20153.0.3.exe",
		"firefox_sha256: 1a79277ac3595d226f40b96c22bc9e9b7d709f614ae77530a58b363644ef4aa9",
		"if: always() && runner.os != 'Windows'",
		"if: always() && runner.os == 'Windows'",
		"rm -rf \"$RUNNER_TEMP/firefox-package\" \"$RUNNER_TEMP/firefox\"",
		"Remove-Item -LiteralPath $package -Recurse -Force -ErrorAction Stop",
		"Remove-Item -LiteralPath $install -Recurse -Force -ErrorAction Stop",
		"go test ./internal/browsercookies/... -count=1 -v",
		"go test ./e2e -run '^TestNativeBrowserNamesRejectInvalidInput$' -count=1",
		"command -v sqlite3\n          sqlite3 --version",
		checkoutAction,
		setupGoAction,
	} {
		if !strings.Contains(workflow, required) {
			return fmt.Errorf("workflow missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"secrets.",
		"environment:",
		"FANBOXSESSID",
		"PIXIV_REFRESH_TOKEN",
		"BROWSER_NATIVE_E2E=1",
		"security find-generic-password",
		"--from-browser",
	} {
		if strings.Contains(workflow, forbidden) {
			return fmt.Errorf("workflow must not contain %q", forbidden)
		}
	}
	for line := range strings.SplitSeq(workflow, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "uses: actions/") && !strings.HasPrefix(trimmed, "- uses: actions/") {
			continue
		}
		at := strings.LastIndex(trimmed, "@")
		if at < 0 || len(trimmed[at+1:]) != 40 || strings.Trim(trimmed[at+1:], "0123456789abcdef") != "" {
			return fmt.Errorf("workflow action is not pinned by a lowercase full SHA: %q", trimmed)
		}
	}
	return nil
}

type workflowFile struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Strategy workflowStrategy `yaml:"strategy"`
}

type workflowStrategy struct {
	Matrix workflowMatrix `yaml:"matrix"`
}

type workflowMatrix struct {
	Include []workflowMatrixEntry `yaml:"include"`
}

type workflowMatrixEntry struct {
	Runner        string `yaml:"runner"`
	GOOS          string `yaml:"goos"`
	GOARCH        string `yaml:"goarch"`
	FirefoxURL    string `yaml:"firefox_url"`
	FirefoxSHA256 string `yaml:"firefox_sha256"`
}

func validateWorkflowMatrices(body []byte) error {
	var document workflowFile
	if err := yaml.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("parse workflow YAML: %w", err)
	}
	browserJob, ok := document.Jobs["browser_provider"]
	if !ok {
		return errors.New("workflow is missing browser_provider job")
	}
	if err := validateMatrix("browser_provider", browserJob.Strategy.Matrix.Include, expectedBrowserMatrix()); err != nil {
		return err
	}
	firefoxJob, ok := document.Jobs["firefox_native"]
	if !ok {
		return errors.New("workflow is missing firefox_native job")
	}
	if err := validateMatrix("firefox_native", firefoxJob.Strategy.Matrix.Include, expectedFirefoxMatrix()); err != nil {
		return err
	}
	return nil
}

func expectedBrowserMatrix() map[string]workflowMatrixEntry {
	return map[string]workflowMatrixEntry{
		"macos-15-intel":   {Runner: "macos-15-intel", GOOS: "darwin", GOARCH: "amd64"},
		"macos-15":         {Runner: "macos-15", GOOS: "darwin", GOARCH: "arm64"},
		"ubuntu-22.04":     {Runner: "ubuntu-22.04", GOOS: "linux", GOARCH: "amd64"},
		"ubuntu-22.04-arm": {Runner: "ubuntu-22.04-arm", GOOS: "linux", GOARCH: "arm64"},
		"windows-2025":     {Runner: "windows-2025", GOOS: "windows", GOARCH: "amd64"},
		"windows-11-arm":   {Runner: "windows-11-arm", GOOS: "windows", GOARCH: "arm64"},
	}
}

func expectedFirefoxMatrix() map[string]workflowMatrixEntry {
	return map[string]workflowMatrixEntry{
		"macos-15-intel": {
			Runner:        "macos-15-intel",
			GOOS:          "darwin",
			GOARCH:        "amd64",
			FirefoxURL:    "https://ftp.mozilla.org/pub/firefox/releases/153.0.3/mac/en-US/Firefox%20153.0.3.dmg",
			FirefoxSHA256: "a0523b6f2f10f13c6071d8b53ed7678193d693febd8a5d4fd8d7417b3c661045",
		},
		"macos-15": {
			Runner:        "macos-15",
			GOOS:          "darwin",
			GOARCH:        "arm64",
			FirefoxURL:    "https://ftp.mozilla.org/pub/firefox/releases/153.0.3/mac/en-US/Firefox%20153.0.3.dmg",
			FirefoxSHA256: "a0523b6f2f10f13c6071d8b53ed7678193d693febd8a5d4fd8d7417b3c661045",
		},
		"ubuntu-22.04": {
			Runner:        "ubuntu-22.04",
			GOOS:          "linux",
			GOARCH:        "amd64",
			FirefoxURL:    "https://ftp.mozilla.org/pub/firefox/releases/153.0.3/linux-x86_64/en-US/firefox-153.0.3.tar.xz",
			FirefoxSHA256: "22b312280900bfb174b685ece32c7b3c6d72e7f8e53d6d30f21ac41a8dc500a2",
		},
		"ubuntu-22.04-arm": {
			Runner:        "ubuntu-22.04-arm",
			GOOS:          "linux",
			GOARCH:        "arm64",
			FirefoxURL:    "https://ftp.mozilla.org/pub/firefox/releases/153.0.3/linux-aarch64/en-US/firefox-153.0.3.tar.xz",
			FirefoxSHA256: "c19b325accedebbc3a1235e3c7104d80c5a4412b368f7d0935b4718114416870",
		},
		"windows-2025": {
			Runner:        "windows-2025",
			GOOS:          "windows",
			GOARCH:        "amd64",
			FirefoxURL:    "https://ftp.mozilla.org/pub/firefox/releases/153.0.3/win64/en-US/Firefox%20Setup%20153.0.3.exe",
			FirefoxSHA256: "8de41917930c35937a46eac6d0e16c633ed7456c771b32b89dc6fd65d55e512e",
		},
		"windows-11-arm": {
			Runner:        "windows-11-arm",
			GOOS:          "windows",
			GOARCH:        "arm64",
			FirefoxURL:    "https://ftp.mozilla.org/pub/firefox/releases/153.0.3/win64-aarch64/en-US/Firefox%20Setup%20153.0.3.exe",
			FirefoxSHA256: "1a79277ac3595d226f40b96c22bc9e9b7d709f614ae77530a58b363644ef4aa9",
		},
	}
}

func validateMatrix(name string, actual []workflowMatrixEntry, expected map[string]workflowMatrixEntry) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("%s matrix has %d entries, want %d", name, len(actual), len(expected))
	}
	seen := make(map[string]struct{}, len(actual))
	for _, entry := range actual {
		if _, duplicate := seen[entry.Runner]; duplicate {
			return fmt.Errorf("%s matrix repeats runner %q", name, entry.Runner)
		}
		want, ok := expected[entry.Runner]
		if !ok || entry != want {
			return fmt.Errorf("%s matrix entry for %q is not the audited target", name, entry.Runner)
		}
		seen[entry.Runner] = struct{}{}
	}
	return nil
}

// runFirefoxContract 在临时用户目录里启动固定发行包一次，让 Firefox 自己生成
// cookies.sqlite/schema；随后只写入合成的 allowlisted cookie，再调用现有 host
// evidence test。这样 CI 可以验证真实 Firefox profile layout，而不会读取用户凭据。
func runFirefoxContract(firefoxPath string) (runErr error) {
	if err := validateFirefoxExecutablePath(firefoxPath); err != nil {
		return err
	}
	goEnvironment, err := currentGoEnvironment(os.Environ())
	if err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "pixiv-cli-firefox-evidence-")
	if err != nil {
		return errors.New("create Firefox evidence directory failed")
	}
	defer func() {
		if cleanupErr := os.RemoveAll(temporary); cleanupErr != nil && runErr == nil {
			runErr = errors.New("remove Firefox evidence directory failed")
		}
	}()

	home := filepath.Join(temporary, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return errors.New("create isolated Firefox home failed")
	}
	dataRoot, err := firefoxDataRootFor(home)
	if err != nil {
		return err
	}
	profileID := "ci.default-release"
	profileDir := filepath.Join(dataRoot, profileID)
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return errors.New("create isolated Firefox profile failed")
	}

	env := isolatedFirefoxEnvironment(os.Environ(), home)
	if err := launchFirefox(firefoxPath, profileDir, temporary, env); err != nil {
		return err
	}
	if err := writeFirefoxProfilesINI(dataRoot, profileID); err != nil {
		return err
	}
	databasePath := filepath.Join(profileDir, "cookies.sqlite")
	if err := requireRegularFile(databasePath); err != nil {
		return errors.New("Firefox did not create cookies.sqlite")
	}
	if err := seedSyntheticFirefoxCookie(databasePath); err != nil {
		return err
	}

	testEnv := setEnvironment(env, "BROWSER_NATIVE_E2E", "1")
	testEnv = setEnvironment(testEnv, "BROWSER_NATIVE_BROWSERS", "firefox")
	testEnv = setEnvironment(testEnv, "BROWSER_NATIVE_PROFILE_FIREFOX", profileID)
	for key, value := range goEnvironment {
		testEnv = setEnvironment(testEnv, key, value)
	}
	repositoryRoot, err := findModuleRoot()
	if err != nil {
		return err
	}
	test := exec.CommandContext(context.Background(), "go", "test", "./e2e", "-run", "^TestRealNativeBrowserProvider$", "-count=1", "-v")
	test.Dir = repositoryRoot
	test.Env = testEnv
	test.Stdout = os.Stdout
	test.Stderr = os.Stderr
	if err := test.Run(); err != nil {
		return errors.New("Firefox provider contract failed")
	}
	return nil
}

// currentGoEnvironment 在替换 Firefox 的 HOME 之前保存 Go 的 cache 路径。
// 临时 profile 不应让嵌套 contract test 重新解析 GOPATH，也不应在离线 runner
// 上把已有 module cache 误判为缺依赖。
func currentGoEnvironment(base []string) (map[string]string, error) {
	command := exec.Command("go", "env", "GOPATH", "GOMODCACHE", "GOCACHE")
	command.Env = base
	output, err := command.Output()
	if err != nil {
		return nil, errors.New("resolve Go cache environment failed")
	}
	lines := strings.Split(strings.TrimRight(string(output), "\r\n"), "\n")
	if len(lines) != 3 {
		return nil, errors.New("Go cache environment is incomplete")
	}
	values := map[string]string{
		"GOPATH":     strings.TrimSuffix(lines[0], "\r"),
		"GOMODCACHE": strings.TrimSuffix(lines[1], "\r"),
		"GOCACHE":    strings.TrimSuffix(lines[2], "\r"),
	}
	for _, value := range values {
		if value == "" {
			return nil, errors.New("Go cache environment contains an empty path")
		}
	}
	return values, nil
}

func findModuleRoot() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", errors.New("find repository root failed")
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root, nil
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "", errors.New("repository root with go.mod was not found")
		}
		root = parent
	}
}

func validateFirefoxExecutablePath(path string) error {
	if path == "" || strings.ContainsAny(path, "\x00\r\n") || !filepath.IsAbs(path) {
		return errors.New("Firefox executable must be an absolute path")
	}
	if err := requireRegularFile(path); err != nil {
		return errors.New("Firefox executable is not a regular file")
	}
	return nil
}

func requireRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("regular file is unavailable")
	}
	return nil
}

func firefoxDataRootFor(home string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Firefox"), nil
	case "linux":
		return filepath.Join(home, ".config", "mozilla", "firefox"), nil
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", "Mozilla", "Firefox"), nil
	default:
		return "", errors.New("Firefox evidence is unsupported on this operating system")
	}
}

func isolatedFirefoxEnvironment(base []string, home string) []string {
	env := append([]string(nil), base...)
	env = setEnvironment(env, "HOME", home)
	env = setEnvironment(env, "XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	env = setEnvironment(env, "USERPROFILE", home)
	env = setEnvironment(env, "APPDATA", filepath.Join(home, "AppData", "Roaming"))
	env = setEnvironment(env, "LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	env = setEnvironment(env, "MOZ_HEADLESS", "1")
	env = setEnvironment(env, "MOZ_CRASHREPORTER_DISABLE", "1")
	return env
}

func setEnvironment(env []string, key, value string) []string {
	prefix := key + "="
	filtered := env[:0]
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, prefix+value)
}

func launchFirefox(firefoxPath, profileDir, temporary string, env []string) error {
	screenshot := filepath.Join(temporary, "firefox-schema.png")
	command := exec.CommandContext(context.Background(), firefoxPath,
		"--headless", "--no-remote", "--profile", profileDir,
		"--screenshot", screenshot, "about:blank")
	command.Env = env
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return errors.New("launch fixed Firefox package failed")
	}
	return nil
}

func writeFirefoxProfilesINI(dataRoot, profileID string) error {
	content := "[General]\nStartWithLastProfile=1\nVersion=2\n\n[Profile0]\nName=browser-native-evidence\nIsRelative=1\nPath=" + profileID + "\nDefault=1\n"
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return errors.New("create Firefox profile root failed")
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "profiles.ini"), []byte(content), 0o600); err != nil {
		return errors.New("write Firefox profiles.ini failed")
	}
	return nil
}

// seedSyntheticFirefoxCookie 只对刚由固定 Firefox 版本生成的临时数据库写入
// 合成值。这里只依赖 Firefox schema 长期稳定、且 provider 查询实际需要的四个
// 核心字段；其他版本字段由 Firefox 自己提供默认值，不能把可选 migration 字段
// 的增删误报成 provider 失败。
func seedSyntheticFirefoxCookie(databasePath string) error {
	const statement = `INSERT OR REPLACE INTO moz_cookies
(name, value, host, path)
VALUES ('FANBOXSESSID', 'browser-native-evidence-synthetic', '.fanbox.cc', '/');`
	command := exec.Command("sqlite3", databasePath, statement)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return errors.New("seed synthetic Firefox cookie failed")
	}
	return nil
}
