// Command licensebundle 从六个 release Rust target 的锁定离线依赖图生成许可证 bundle。
package main

import (
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/licensebundle"
)

func main() {
	if err := licensebundle.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "license bundle:", err)
		os.Exit(1)
	}
}
