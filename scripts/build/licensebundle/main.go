// Command licensebundle 从六个平台的 Go 生产依赖及移植来源生成许可证 bundle。
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "license bundle:", err)
		os.Exit(1)
	}
}
