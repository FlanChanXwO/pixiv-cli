// Command releasenotes audits and validates the versioned changelog contract.
package main

import (
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/releasenotes"
)

func main() {
	if err := releasenotes.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
