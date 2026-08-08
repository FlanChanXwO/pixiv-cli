package filesystem

// Replace 用同平台的原子替换原语提交 sourcePath 到 targetPath。
// 具体实现由各平台文件负责；调用方必须先完成源文件写入与关闭。
func Replace(sourcePath, targetPath string) error {
	return ReplaceFile(sourcePath, targetPath)
}
