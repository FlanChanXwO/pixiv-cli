// Command nativeevidence records and validates non-release native runner evidence.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "native evidence: %v\n", err)
		os.Exit(1)
	}
}
