// Command frontendassets generates the pages embedded in the single-file CLI.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/frontendassets"
)

func main() {
	check := flag.Bool("check", false, "verify generated assets without writing files")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "frontend assets: unexpected arguments")
		os.Exit(1)
	}
	if err := frontendassets.Generate(".", *check); err != nil {
		fmt.Fprintf(os.Stderr, "frontend assets: %v\n", err)
		os.Exit(1)
	}
}
