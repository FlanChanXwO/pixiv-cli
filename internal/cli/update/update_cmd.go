package update

import (
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate"
	"github.com/FlanChanXwO/pixiv-cli/internal/version"
	"github.com/spf13/cobra"
)

type commandOptions struct {
	check             bool
	includePrerelease bool
	jsonOut           bool
	proxy             string
}

// New 构造 update 命令；loadRuntime 提供配置中的默认代理。
func New(env command.Env, loadRuntime func() (settings.RuntimeConfig, error)) *cobra.Command {
	var opts commandOptions
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Check for or install updates",
		Example: "pixiv update --check",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := env.ExactArgs(0, "pixiv update [--check] [--prerelease] [--proxy URL]")(cmd, args); err != nil {
				return err
			}
			if opts.jsonOut && !opts.check {
				return fmt.Errorf("--json is only supported with --check")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			runtimeConfig, err := loadRuntime()
			if err != nil {
				return fmt.Errorf("load update configuration: %w", err)
			}
			proxy := runtimeConfig.HTTPSProxy
			if cmd.Flags().Changed("proxy") {
				proxy = opts.proxy
			}
			coordinator, err := NewCoordinator(proxy, env.Out, env.Err)
			if err != nil {
				return err
			}
			result, err := coordinator.Execute(cmd.Context(), selfupdate.UpdateRequest{
				BuildInfo:         version.Current(),
				Check:             opts.check,
				IncludePrerelease: opts.includePrerelease,
			})
			if err != nil {
				return err
			}
			if opts.jsonOut {
				return env.PrintJSON(result)
			}
			return printResult(env.Out, result)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.check, "check", false, "check for an update without installing it")
	flags.BoolVar(&opts.includePrerelease, "prerelease", false, "include prerelease updates")
	flags.BoolVar(&opts.jsonOut, "json", false, "print update check status as JSON (requires --check)")
	flags.StringVar(&opts.proxy, "proxy", "", "HTTP(S) proxy URL for this update command")
	input.Bind(cmd, input.InputSpec{Codec: input.NoInput, MinArgs: 0, MaxArgs: 0})
	command.Bind(cmd, command.UpdateCommand())
	return cmd
}

func printResult(out io.Writer, result selfupdate.UpdateResult) error {
	latestVersion := "none"
	if result.LatestVersion != nil {
		latestVersion = *result.LatestVersion
	}
	_, err := fmt.Fprintf(out, "source: %s\ncurrent version: %s\nlatest version: %s\nupdate available: %s\n", result.Source, result.CurrentVersion, latestVersion, command.TextBool(result.UpdateAvailable))
	return err
}
