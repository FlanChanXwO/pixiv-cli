package main

import (
	"os"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
