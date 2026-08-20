package main

import (
	"context"
	"os"
	"os/signal"

	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli"
)

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

// run 将 Ctrl+C 作为根 context 传入 CLI；defer stop 解除 signal 包安装的处理器，
// 使嵌入式测试和未来重复调用不会遗留 handler。CLI runtime 在 Run 返回前显式关闭。
func run(args []string, in *os.File, out, errOut *os.File) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return pixivcmd.RunContextWithDefaultBrokenPipeSignals(ctx, args, in, out, errOut)
}
