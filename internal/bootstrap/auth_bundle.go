package bootstrap

import "github.com/FlanChanXwO/pixiv-cli/internal/filesystem"

// WriteAuthExportBundle 是 CLI auth export 的唯一生产文件写入入口；凭据
// 文件的私有权限、独占创建和原子替换由 filesystem 统一维护。
func WriteAuthExportBundle(path string, body []byte, force bool) error {
	return filesystem.WriteSecretFile(path, body, force)
}
