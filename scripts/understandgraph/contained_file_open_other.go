//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package main

import "os"

func openContainedFile(path string) (*os.File, error) {
	return os.Open(path)
}
