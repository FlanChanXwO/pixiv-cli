//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package main

import (
	"os/signal"
	"syscall"
)

// enablePipelineBrokenPipeSignal 仅在普通 CLI 的 NDJSON 输出命令期间暂时忽略
// SIGPIPE，使写操作返回 EPIPE 交由退出码契约处理；返回函数恢复默认处理。
func enablePipelineBrokenPipeSignal() func() {
	return enableBrokenPipeSignal()
}

// enableMCPBrokenPipeSignal 只在 MCP stdio transport 运行期间启用，使 JSON-RPC
// 写失败回到 Go 的错误链，最终以普通失败退出；它绝不使用 NDJSON 的成功语义。
func enableMCPBrokenPipeSignal() func() {
	return enableBrokenPipeSignal()
}

func enableBrokenPipeSignal() func() {
	signal.Ignore(syscall.SIGPIPE)
	return func() { signal.Reset(syscall.SIGPIPE) }
}
