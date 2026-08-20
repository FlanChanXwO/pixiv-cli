// Command releaseassets assembles the deterministic artifacts attached to a Pixiv CLI release.
package main

import (
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/releaseassets"
)

func main() {
	if err := releaseassets.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "release assets: %v\n", err)
		os.Exit(1)
	}
}
