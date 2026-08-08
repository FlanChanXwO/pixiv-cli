//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package filesystem

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// acquire 故意阻塞等待原生锁；没有人为超时，避免合法持久状态事务被伪装成失败。
func acquire(file *os.File) (func() error, error) {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return nil, err
	}
	return func() error {
		return errors.Join(unix.Flock(int(file.Fd()), unix.LOCK_UN), file.Close())
	}, nil
}
