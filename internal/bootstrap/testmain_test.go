package bootstrap

import (
	"os"
	"testing"
)

// TestMain 隔离整个 bootstrap 包测试的 HOME，使 Runtime 打开的鉴权
// 数据库只落在临时目录，绝不写入开发者宿主机的 ~/.pixiv-cli。
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "pixiv-cli-bootstrap-*")
	if err != nil {
		panic(err)
	}
	oldHome, hadHome := os.LookupEnv("HOME")
	oldUserProfile, hadUserProfile := os.LookupEnv("USERPROFILE")
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	if err := os.Setenv("USERPROFILE", home); err != nil {
		panic(err)
	}
	code := m.Run()
	if hadHome {
		_ = os.Setenv("HOME", oldHome)
	} else {
		_ = os.Unsetenv("HOME")
	}
	if hadUserProfile {
		_ = os.Setenv("USERPROFILE", oldUserProfile)
	} else {
		_ = os.Unsetenv("USERPROFILE")
	}
	_ = os.RemoveAll(home)
	os.Exit(code)
}
