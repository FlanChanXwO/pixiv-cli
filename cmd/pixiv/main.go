package main

import (
	"os"

	pixivcmd "github.com/FlanChanXwO/pixiv-mcp-server/internal/cli"
)

func main() {
	os.Exit(pixivcmd.Run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
