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

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/system"
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
	return verifySyntheticFirefoxProviderContract(home, profileID)
}

// verifySyntheticFirefoxProviderContract 直接通过正式 provider 读取隔离 profile，
// 避免 browser evidence 为一个 provider contract 编译整个 e2e package 及无关媒体/cgo 链。
func verifySyntheticFirefoxProviderContract(home, profileID string) (runErr error) {
	isolated := map[string]string{
		"HOME":            home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"USERPROFILE":     home,
		"APPDATA":         filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA":    filepath.Join(home, "AppData", "Local"),
	}
	type originalValue struct {
		value string
		set   bool
	}
	original := make(map[string]originalValue, len(isolated))
	for key := range isolated {
		previous, set := os.LookupEnv(key)
		original[key] = originalValue{value: previous, set: set}
	}
	defer func() {
		for key, previous := range original {
			var err error
			if previous.set {
				err = os.Setenv(key, previous.value)
			} else {
				err = os.Unsetenv(key)
			}
			if err != nil && runErr == nil {
				runErr = errors.New("restore firefox environment failed")
			}
		}
	}()
	for key, value := range isolated {
		if err := os.Setenv(key, value); err != nil {
			return errors.New("set isolated firefox environment failed")
		}
	}

	provider, err := system.New("firefox")
	if err != nil {
		return errors.New("create firefox provider failed")
	}
	defer func() {
		if err := provider.Close(); err != nil && runErr == nil {
			runErr = errors.New("close firefox provider failed")
		}
	}()

	ctx := context.Background()
	profiles, err := provider.DiscoverProfiles(ctx)
	if err != nil {
		return errors.New("discover firefox profile failed")
	}
	profile, err := system.SelectProfile(profiles, profileID)
	if err != nil {
		return errors.New("select firefox profile failed")
	}
	secrets, err := provider.Read(ctx, system.DefaultQuery, profile.ID)
	if err != nil {
		return errors.New("read synthetic firefox cookie failed")
	}
	if len(secrets) != 1 || secrets[0].Value() != syntheticFirefoxCookieValue {
		return errors.New("firefox provider contract returned an invalid allowlisted cookie set")
	}
	return nil
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
const syntheticFirefoxCookieValue = "browser-native-evidence-synthetic"

func seedSyntheticFirefoxCookie(databasePath string) error {
	const statement = `INSERT OR REPLACE INTO moz_cookies
(name, value, host, path)
VALUES ('FANBOXSESSID', '` + syntheticFirefoxCookieValue + `', '.fanbox.cc', '/');`
	command := exec.Command("sqlite3", databasePath, statement)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return errors.New("seed synthetic Firefox cookie failed")
	}
	return nil
}
