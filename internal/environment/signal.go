package environment

import (
	"os/signal"
	"syscall"
)

// BrokenPipe 临时安装平台的 broken-pipe 信号策略，使 stdout 写失败能以 EPIPE 返回
// Go 调用链。NDJSON 与 MCP 各持有一个状态，只有前者可把 EPIPE 归为成功。
type BrokenPipe struct{ stop func() }

// Enable 忽略 SIGPIPE 并记住恢复函数；重复调用不产生额外副作用。
func (b *BrokenPipe) Enable() {
	if b.stop != nil {
		return
	}
	signal.Ignore(syscall.SIGPIPE)
	b.stop = func() { signal.Reset(syscall.SIGPIPE) }
}

// Installed 报告当前是否安装了 broken-pipe 策略。
func (b *BrokenPipe) Installed() bool { return b.stop != nil }

// Restore 在本次执行结束时恢复平台默认处理；重复调用是安全的。
func (b *BrokenPipe) Restore() {
	if b.stop == nil {
		return
	}
	b.stop()
	b.stop = nil
}
