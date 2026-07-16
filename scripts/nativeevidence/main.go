// Command nativeevidence records and validates non-release native runner evidence.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "native evidence: %v\n", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("usage: nativeevidence policy|record|consolidate")
	}
	switch arguments[0] {
	case "policy":
		return runPolicy(arguments[1:])
	case "record":
		return runRecord(arguments[1:])
	case "consolidate":
		return runConsolidate(arguments[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", arguments[0])
	}
}

func runPolicy(arguments []string) error {
	if len(arguments) != 2 || arguments[0] != "--workflow" {
		return errors.New("usage: nativeevidence policy --workflow PATH")
	}
	body, err := os.ReadFile(arguments[1])
	if err != nil {
		return fmt.Errorf("read workflow: %w", err)
	}
	return checkWorkflow(body)
}

func runRecord(arguments []string) error {
	flags := flag.NewFlagSet("record", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := recordOptions{}
	flags.StringVar(&options.repoRoot, "repo-root", "", "repository root")
	flags.StringVar(&options.version, "version", "", "semantic version without v")
	flags.StringVar(&options.target, "target", "", "GOOS/GOARCH target")
	flags.StringVar(&options.rustTarget, "rust-target", "", "Rust target triple")
	flags.StringVar(&options.staticlib, "staticlib", "", "native Rust static library")
	flags.StringVar(&options.binary, "binary", "", "versioned pixiv binary")
	flags.StringVar(&options.archive, "archive", "", "release-style archive")
	flags.StringVar(&options.output, "output", "", "new evidence JSON path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("record accepts no positional arguments: %q", flags.Arg(0))
	}
	_, err := recordEvidence(options)
	return err
}

func runConsolidate(arguments []string) error {
	flags := flag.NewFlagSet("consolidate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := consolidateOptions{}
	flags.StringVar(&options.repoRoot, "repo-root", "", "repository root matching the audited evidence source")
	flags.StringVar(&options.expectedVersion, "expected-version", "", "exact v-prefixed binary version from the audited main workflow run")
	flags.StringVar(&options.expectedCommit, "expected-commit", "", "exact main commit SHA from the audited workflow run")
	flags.StringVar(&options.inputDir, "input-dir", "", "directory containing downloaded native evidence artifacts")
	flags.StringVar(&options.outputDir, "output-dir", "", "new staticlib directory receiving a complete manifest")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("consolidate accepts no positional arguments: %q", flags.Arg(0))
	}
	return consolidateEvidence(options)
}
