package installer

import (
	"fmt"
	"os"
	"runtime"
)

// CleanupPendingWindowsUpdate 删除 ReplaceFileW 留下的旧可执行文件。非 Windows 不创建
// 该备份，因此没有任何文件操作；调用方应在启动 CLI 前调用并把失败展示给用户。
func CleanupPendingWindowsUpdate() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable for pending update cleanup: %w", err)
	}
	executable, err = resolveReleaseExecutablePath(executable)
	if err != nil {
		return err
	}
	oldPath := executable + ".old"
	if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pending old executable %q: %w", oldPath, err)
	}
	return nil
}
