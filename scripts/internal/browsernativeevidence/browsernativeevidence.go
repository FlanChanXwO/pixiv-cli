// Package browsernativeevidence 运行无 credential 的 browser provider evidence helper。
package browsernativeevidence

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
)

// Run 是 scripts/cmd/browsernativeevidence 的入口 owner：解析参数并映射 exit code。
func Run(args []string) int {
	switch {
	case len(args) == 3 && args[0] == "firefox-contract" && args[1] == "--firefox":
		if err := runFirefoxContract(args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "browser evidence: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: browsernativeevidence firefox-contract --firefox PATH")
		return 2
	}
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
