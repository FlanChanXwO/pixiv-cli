// Command releaseworkflow 检查发布 workflow 的结构化安全与质量门禁。
package main

import (
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/releaseworkflow"
)

func main() {
	if err := releaseworkflow.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "release workflow policy: %v\n", err)
		os.Exit(1)
	}
}
