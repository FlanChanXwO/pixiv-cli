//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package localstate

import (
	"errors"
	"os"
)

func acquire(*os.File) (func() error, error) {
	return nil, errors.New("local state locking is unsupported on this platform")
}
