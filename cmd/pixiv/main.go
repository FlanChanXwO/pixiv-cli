package main

import (
	"os"

	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli"
)

func main() {
	os.Exit(pixivcmd.Run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
