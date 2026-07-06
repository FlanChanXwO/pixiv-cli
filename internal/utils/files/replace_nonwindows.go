//go:build !windows

package files

import "os"

// ReplaceFile 用源文件替换目标路径；调用方应确保源文件已完整写入并关闭。
func ReplaceFile(sourcePath, targetPath string) error {
	return os.Rename(sourcePath, targetPath)
}
