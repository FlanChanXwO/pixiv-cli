package update

import (
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
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
func New(env environment.Env, loadRuntime func() (settings.RuntimeConfig, error)) *cobra.Command {
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
			return printResult(env.Out, result, opts.check)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.check, "check", false, "check for an update without installing it")
	flags.BoolVar(&opts.includePrerelease, "prerelease", false, "include prerelease updates")
	flags.BoolVar(&opts.jsonOut, "json", false, "print update check status as JSON (requires --check)")
	flags.StringVar(&opts.proxy, "proxy", "", "HTTP(S) proxy URL for this update command")
	environment.BindInput(cmd, environment.InputSpec{Codec: environment.NoInput, MinArgs: 0, MaxArgs: 0})
	environment.Bind(cmd, environment.UpdateCommand())
	return cmd
}

func printResult(out io.Writer, result selfupdate.UpdateResult, check bool) error {
	if result.UpdateAvailable && result.LatestVersion != nil {
		if !check {
			_, err := fmt.Fprintf(out, "Updated pixiv to %s\n", *result.LatestVersion)
			return err
		}
		_, err := fmt.Fprintf(out, "Update available: %s → %s (run \"pixiv update\")\n", result.CurrentVersion, *result.LatestVersion)
		return err
	}
	_, err := fmt.Fprintf(out, "pixiv %s is up to date\n", result.CurrentVersion)
	return err
}
