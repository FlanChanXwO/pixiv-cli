//go:build windows

package files

// syncParentDirectory 在 Windows 上不伪造 POSIX directory fsync 保证。调用方仍会在
// ReplaceFileW/MoveFileEx 前同步并关闭临时文件，但 Go/Win32 没有等价的目录句柄同步路径。
func syncParentDirectory(string) error {
	return nil
}
