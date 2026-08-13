// Command linuxabi 验证已打包 Linux executable 是否保持公开的 glibc 基线。
package main

import (
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/linuxabi"
)

func main() {
	if err := linuxabi.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
