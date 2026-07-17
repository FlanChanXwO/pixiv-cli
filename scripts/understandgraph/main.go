package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("command is required (supported: normalize)")
	}
	if args[0] != "normalize" {
		return fmt.Errorf("unknown command %q (supported: normalize)", args[0])
	}
	flags := flag.NewFlagSet("normalize", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "repository root containing .understand-anything")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *root == "" {
		return errors.New("--root is required")
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	return Normalize(*root)
}
