// Command browsernativeevidence 运行 browser provider evidence helper。
package main

import (
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/browsernativeevidence"
)

func main() {
	os.Exit(browsernativeevidence.Run(os.Args[1:]))
}
