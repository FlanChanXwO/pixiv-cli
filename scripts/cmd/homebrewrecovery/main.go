// Command homebrewrecovery decides whether a Homebrew tap may accept a new
// Formula, so recovery of the same release_run_id is idempotent and an older
// run can never roll the published Formula back.
package main

import (
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/homebrewrecovery"
)

func main() {
	if err := homebrewrecovery.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "homebrew recovery: %v\n", err)
		os.Exit(1)
	}
}
