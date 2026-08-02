package bootstrap

import (
	"os"
	"testing"
)

// TestMain 隔离整个 bootstrap 包测试的 HOME，使 NewServices 打开的 FANBOX 鉴权
// 数据库只落在临时目录，绝不写入开发者宿主机的 ~/.pixiv-cli。
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "pixiv-cli-bootstrap-*")
	if err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
