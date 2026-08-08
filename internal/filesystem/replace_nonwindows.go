//go:build !windows

package filesystem

import "os"

// ReplaceFile 用源文件替换目标路径；调用方应确保源文件已完整写入并关闭。
func ReplaceFile(sourcePath, targetPath string) error {
	return os.Rename(sourcePath, targetPath)
}

// ReplaceFileWithBackup 与 Windows 的 API 形状一致；非 Windows 的 rename 已原子替换目标，
// 不会产生旧文件备份。
func ReplaceFileWithBackup(sourcePath, targetPath, _ string) error {
	return ReplaceFile(sourcePath, targetPath)
}
