// Command homebrewformula 根据已验证 release checksums.txt 的六个 archive
// 渲染 URL 与 digest 均受约束的 Homebrew formula。
package main

import (
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/homebrewformula"
)

func main() {
	if err := homebrewformula.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "homebrew formula: %v\n", err)
		os.Exit(1)
	}
}
