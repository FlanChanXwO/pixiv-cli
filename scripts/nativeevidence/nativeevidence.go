// native runner evidence 的 record/consolidate 子命令分发；入口在 main.go。

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("usage: nativeevidence record|consolidate")
	}
	switch arguments[0] {
	case "record":
		return runRecord(arguments[1:])
	case "consolidate":
		return runConsolidate(arguments[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", arguments[0])
	}
}

func runRecord(arguments []string) error {
	flags := flag.NewFlagSet("record", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	options := recordOptions{}
	flags.StringVar(&options.repoRoot, "repo-root", "", "repository root")
	flags.StringVar(&options.version, "version", "", "semantic version without v")
	flags.StringVar(&options.sourceCommit, "source-commit", "", "exact source commit SHA for this workflow run")
	flags.StringVar(&options.target, "target", "", "GOOS/GOARCH target")
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
	flags.StringVar(&options.expectedVersion, "expected-version", "", "exact v-prefixed binary version from the audited workflow run")
	flags.StringVar(&options.expectedCommit, "expected-commit", "", "exact source commit SHA from the audited workflow run")
	flags.StringVar(&options.inputDir, "input-dir", "", "directory containing downloaded native evidence artifacts")
	flags.StringVar(&options.outputDir, "output-dir", "", "new directory receiving six complete Go evidence bundles")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("consolidate accepts no positional arguments: %q", flags.Arg(0))
	}
	return consolidateEvidence(options)
}
