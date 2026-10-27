// pixiv 是 Pixiv 与 FANBOX 的命令行入口。它从 main 转发参数、
// 标准流和进程信号，命令树与运行时的构造在 internal/cli。
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli"
)

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

// run 把中断与终止信号转成根 context 后交给 CLI。
// defer stop 解除 signal 安装的处理器，嵌入式调用不会遗留 handler；
// CLI runtime 在 Run 返回前自行关闭。
func run(args []string, in *os.File, out, errOut *os.File) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cli.Run(ctx, args, in, out, errOut)
}
