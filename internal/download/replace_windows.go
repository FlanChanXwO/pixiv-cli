//go:build windows

package download

import "golang.org/x/sys/windows"

func replaceDownloadedFile(tmpPath, targetPath string) error {
	from, err := windows.UTF16PtrFromString(tmpPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}
	// Windows 的 os.Rename 不能替换已存在目标；MoveFileEx 提供同目录替换语义。
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING)
}
