// Command pr owns the trusted PR gates: metadata validation, /test trigger and
// command resolution, plus cross-platform verification execution and rendering.

package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fatal(fmt.Errorf("expected a PR subcommand: validate, trigger, resolve, init, execute, or render"))
	}
	switch os.Args[1] {
	case "validate":
		runMeta(os.Args[2:])
	case "trigger":
		runTrigger(os.Args[2:])
	case "resolve":
		runResolve(os.Args[2:])
	case "init", "execute", "render":
		runVerify(os.Args[1:])
	default:
		fatal(fmt.Errorf("unknown PR subcommand %q", os.Args[1]))
	}
}
