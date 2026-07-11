package cli

import (
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/spf13/cobra"
)

var (
	loadUpdateRuntimeConfig     = bootstrap.LoadRuntimeConfig
	newUpdateCommandCoordinator = bootstrap.NewUpdateCoordinator
)

type updateOptions struct {
	check             bool
	includePrerelease bool
	jsonOut           bool
	proxy             string
}

func (a app) newUpdateCommand() *cobra.Command {
	var opts updateOptions
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Check for or install updates",
		Example: "pixiv update --check",
		Args:    requireExactArgs(0, "pixiv update [--check] [--prerelease] [--proxy URL]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// --json 的输出契约只覆盖只读检查，执行更新时不可混入机器可读状态与子进程输出。
			if opts.jsonOut && !opts.check {
				return fmt.Errorf("--json is only supported with --check")
			}
			runtimeConfig, err := loadUpdateRuntimeConfig()
			if err != nil {
				return fmt.Errorf("load update configuration: %w", err)
			}
			proxy := runtimeConfig.HTTPSProxy
			if cmd.Flags().Changed("proxy") {
				proxy = opts.proxy
			}
			coordinator, err := newUpdateCommandCoordinator(proxy, a.out, a.errOut)
			if err != nil {
				return err
			}
			result, err := coordinator.Execute(cmd.Context(), update.UpdateRequest{
				BuildInfo:         buildinfo.Current(),
				Check:             opts.check,
				IncludePrerelease: opts.includePrerelease,
			})
			if err != nil {
				return err
			}
			if opts.jsonOut {
				return a.printJSON(result)
			}
			return printUpdateResult(a.out, result)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.check, "check", false, "check for an update without installing it")
	flags.BoolVar(&opts.includePrerelease, "prerelease", false, "include prerelease updates")
	flags.BoolVar(&opts.jsonOut, "json", false, "print update check status as JSON (requires --check)")
	flags.StringVar(&opts.proxy, "proxy", "", "HTTP(S) proxy URL for this update command")
	return cmd
}

func printUpdateResult(out io.Writer, result update.UpdateResult) error {
	latestVersion := "none"
	if result.LatestVersion != nil {
		latestVersion = *result.LatestVersion
	}
	_, err := fmt.Fprintf(out, "source: %s\ncurrent version: %s\nlatest version: %s\nupdate available: %s\n", result.Source, result.CurrentVersion, latestVersion, textBool(result.UpdateAvailable))
	return err
}
