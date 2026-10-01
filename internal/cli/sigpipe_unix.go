//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import (
	"os/signal"
	"syscall"
)

// enablePipelineBrokenPipeSignal 仅在普通 CLI 的 NDJSON 输出命令期间暂时忽略
// SIGPIPE，使写操作返回 EPIPE 交由退出码契约处理；返回函数恢复默认处理。
func enablePipelineBrokenPipeSignal() func() {
	signal.Ignore(syscall.SIGPIPE)
	return func() { signal.Reset(syscall.SIGPIPE) }
}
