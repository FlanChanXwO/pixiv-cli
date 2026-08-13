package release

import "context"

// ReleaseCache 是 GitHub Releases 缓存的后端存储端口。读取时第二个返回值
// 报告条目是否已存在；写入必须原子完成并保持私密权限。
type ReleaseCache interface {
	Read(ctx context.Context) ([]byte, bool, error)
	Write(ctx context.Context, data []byte) error
}
