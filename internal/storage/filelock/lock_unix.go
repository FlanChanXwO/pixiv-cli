//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func tryAcquire(file *os.File) (func() error, bool, error) {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() error { return errors.Join(unix.Flock(int(file.Fd()), unix.LOCK_UN), file.Close()) }, true, nil
}

// 不可取消的context沿用原生等待；可取消等待由通用层处理，不设置事务超时。
func acquire(file *os.File) (func() error, error) {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		return nil, err
	}
	return func() error {
		return errors.Join(unix.Flock(int(file.Fd()), unix.LOCK_UN), file.Close())
	}, nil
}
