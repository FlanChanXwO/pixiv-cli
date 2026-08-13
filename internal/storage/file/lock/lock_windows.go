//go:build windows

package lock

import (
	"errors"
	"os"
	"runtime"

	"golang.org/x/sys/windows"
)

func acquire(file *os.File) (func() error, error) {
	var overlap windows.Overlapped
	handle := windows.Handle(file.Fd())
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlap); err != nil {
		return nil, err
	}
	runtime.KeepAlive(file)
	return func() error {
		err := windows.UnlockFileEx(handle, 0, 1, 0, &overlap)
		runtime.KeepAlive(file)
		return errors.Join(err, file.Close())
	}, nil
}
