// Command nativeevidence records and validates non-release native runner evidence.
package main

import (
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/nativeevidence"
)

func main() {
	if err := nativeevidence.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "native evidence: %v\n", err)
		os.Exit(1)
	}
}
