package main

import (
	"fmt"
	"os"

	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
)

func main() {
	if err := update.CleanupPendingWindowsUpdate(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "clean pending update: %v\n", err)
		os.Exit(1)
	}
	os.Exit(pixivcmd.Run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
