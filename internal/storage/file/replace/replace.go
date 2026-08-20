package replace

// Result 描述 private-file 替换在返回错误时留下的文件状态。
// Committed 表示 target 已经换成 source；PreserveSource 表示替换状态未决，
// source 仍是恢复材料，调用方不得将其当作普通失败临时文件删除。
type Result struct {
	Committed      bool
	PreserveSource bool
}

// ReplacePrivateFile 使用本平台的 private-file 替换协议。
func ReplacePrivateFile(sourcePath, targetPath string) (Result, error) {
	return replacePrivateFile(sourcePath, targetPath)
}

// SyncParentDirectory 在替换提交后同步目标目录项。Windows 上由平台实现返回
// 可兑现的保证；调用方仍需处理返回的真实错误。
func SyncParentDirectory(path string) error {
	return syncParentDirectory(path)
}

// Replace 用同平台的原子替换原语提交 sourcePath 到 targetPath。
// 具体实现由各平台文件负责；调用方必须先完成源文件写入与关闭。
func Replace(sourcePath, targetPath string) error {
	return ReplaceFile(sourcePath, targetPath)
}
