//go:build !windows

package replace

import (
	"errors"
	"os"
)

// syncParentDirectory 在原子替换后同步目录项，使 rename 在 Unix-like 文件系统上具备
// 对应的持久化保证。替换已提交后若同步失败，调用方会收到真实错误，不能假称已回滚。
func syncParentDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}
