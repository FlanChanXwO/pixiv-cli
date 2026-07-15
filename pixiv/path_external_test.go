package pixiv_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// 顶层路径是外部 Go 调用方唯一支持的 SDK 导入入口。
func TestTopLevelPackageConstructsClient(t *testing.T) {
	t.Parallel()

	client, err := pixiv.NewClient(pixiv.Options{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil client")
	}
}
