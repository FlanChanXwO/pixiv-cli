//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package main

import (
	"os"
	"syscall"
)

func openContainedFile(path string) (*os.File, error) {
	// 非阻塞打开确保校验与 open 之间若被替换成 FIFO，也能先由 fd.Stat 拒绝而不会挂住归一化。
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
