// Command prepublishhomebrew 检查 Homebrew 发布前验证与受保护恢复 workflow 的安全边界。
package main

import (
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/prepublishhomebrew"
)

func main() {
	if err := prepublishhomebrew.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "prepublish Homebrew workflow policy: %v\n", err)
		os.Exit(1)
	}
}
