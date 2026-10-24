// Command homebrewrecovery decides whether a Homebrew tap may accept a new
// Formula, so recovery of the same release_run_id is idempotent and an older
// run can never roll the published Formula back.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "homebrew recovery: %v\n", err)
		os.Exit(1)
	}
}
