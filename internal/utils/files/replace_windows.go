//go:build windows

package files

import "golang.org/x/sys/windows"

// ReplaceFile 用源文件替换目标路径；Windows 下需要显式允许替换已存在目标。
func ReplaceFile(sourcePath, targetPath string) error {
	from, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING)
}
