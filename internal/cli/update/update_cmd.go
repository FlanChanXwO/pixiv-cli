package update

import (
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	updateapp "github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/spf13/cobra"
)

type commandOptions struct {
	check             bool
	includePrerelease bool
	jsonOut           bool
	proxy             string
}

func newUpdateCommand(host Host) *cobra.Command {
	var opts commandOptions
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Check for or install updates",
		Example: "pixiv update --check",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := host.RequireExactArgs(0, "pixiv update [--check] [--prerelease] [--proxy URL]")(cmd, args); err != nil {
				return err
			}
			if opts.jsonOut && !opts.check {
				return fmt.Errorf("--json is only supported with --check")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			runtimeConfig, err := host.LoadUpdateRuntimeConfig()
			if err != nil {
				return fmt.Errorf("load update configuration: %w", err)
			}
			proxy := runtimeConfig.HTTPSProxy
			if cmd.Flags().Changed("proxy") {
				proxy = opts.proxy
			}
			coordinator, err := host.NewUpdateCoordinator(proxy, host.Output(), host.ErrorOutput())
			if err != nil {
				return err
			}
			result, err := coordinator.Execute(cmd.Context(), updateapp.UpdateRequest{
				BuildInfo:         buildinfo.Current(),
				Check:             opts.check,
				IncludePrerelease: opts.includePrerelease,
			})
			if err != nil {
				return err
			}
			if opts.jsonOut {
				return host.PrintJSON(result)
			}
			return printResult(host.Output(), result)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.check, "check", false, "check for an update without installing it")
	flags.BoolVar(&opts.includePrerelease, "prerelease", false, "include prerelease updates")
	flags.BoolVar(&opts.jsonOut, "json", false, "print update check status as JSON (requires --check)")
	flags.StringVar(&opts.proxy, "proxy", "", "HTTP(S) proxy URL for this update command")
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.NoInput, MinArgs: 0, MaxArgs: 0})
	requirements.Bind(cmd, requirements.UpdateCommand())
	return cmd
}

func printResult(out io.Writer, result updateapp.UpdateResult) error {
	latestVersion := "none"
	if result.LatestVersion != nil {
		latestVersion = *result.LatestVersion
	}
	_, err := fmt.Fprintf(out, "source: %s\ncurrent version: %s\nlatest version: %s\nupdate available: %s\n", result.Source, result.CurrentVersion, latestVersion, textBool(result.UpdateAvailable))
	return err
}

func textBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
