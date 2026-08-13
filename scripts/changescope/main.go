// Command changescope classifies a Git diff for GitHub Actions CI routing.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/changescope"
)

func main() {
	base := flag.String("base", "", "base Git commit")
	head := flag.String("head", "", "head Git commit")
	githubOutput := flag.String("github-output", "", "GitHub Actions output file")
	flag.Parse()

	docsOnly, reason, err := changescope.Classify(*base, *head)
	if err != nil {
		fmt.Fprintf(os.Stderr, "classify change scope: %v\n", err)
		os.Exit(1)
	}
	if err := changescope.WriteOutput(*githubOutput, docsOnly); err != nil {
		fmt.Fprintf(os.Stderr, "write change scope output: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, reason)
	fmt.Printf("docs_only=%t\n", docsOnly)
}
