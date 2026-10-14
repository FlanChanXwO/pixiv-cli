// Command frontendassets validates embedded sources without writing generated files.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/frontend"
)

func main() {
	flag.Bool("check", false, "validate embedded sources (also the default)")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "frontend assets: unexpected arguments")
		os.Exit(1)
	}
	if _, err := frontend.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "frontend assets: %v\n", err)
		os.Exit(1)
	}
}
