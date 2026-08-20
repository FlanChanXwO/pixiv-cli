// Command browsernativeevidence 校验无 credential 的 browser provider workflow。
package main

import (
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/browsernativeevidence"
)

func main() {
	os.Exit(browsernativeevidence.Run(os.Args[1:]))
}
