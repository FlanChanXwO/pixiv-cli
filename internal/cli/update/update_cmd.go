package update

import (
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
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
func New(env environment.Env, loadRuntime func() (config.Config, error)) *cobra.Command {
	var opts commandOptions
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "help.check_for_or_install_updates",
		Example: "pixiv update --check",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := env.ExactArgs(0, "pixiv update [--check] [--prerelease] [--proxy URL]")(cmd, args); err != nil {
				return err
			}
			if opts.jsonOut && !opts.check {
				return i18n.Errorf("error.json_is_only_supported_with_check")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			runtimeConfig, err := loadRuntime()
			if err != nil {
				return i18n.Errorf("error.load_update_configuration", err)
			}
			proxy := runtimeConfig.Network.HTTPSProxy
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
			return printResult(env.Out, env.Language, result, opts.check)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.check, "check", false, "flag.check_for_an_update_without_installing_it")
	flags.BoolVar(&opts.includePrerelease, "prerelease", false, "flag.include_prerelease_updates")
	flags.BoolVar(&opts.jsonOut, "json", false, "flag.print_update_check_status_as_json_requires_check")
	flags.StringVar(&opts.proxy, "proxy", "", "flag.http_s_proxy_url_for_this_update_command")
	environment.BindInput(cmd, environment.InputSpec{Codec: environment.NoInput, MinArgs: 0, MaxArgs: 0})
	environment.Bind(cmd, environment.UpdateCommand())
	return cmd
}

func printResult(out io.Writer, language i18n.Locale, result selfupdate.UpdateResult, check bool) error {
	if result.UpdateAvailable && result.LatestVersion == nil {
		if !check {
			_, err := fmt.Fprintln(out, language.Text("output.switched_pixiv_homebrew_channel"))
			return err
		}
		command := "pixiv update"
		if result.Source == selfupdate.InstallSourceHomebrewStable {
			command += " --prerelease"
		}
		_, err := fmt.Fprintf(out, language.Text("output.update_available_switch_homebrew_channel_run"), command)
		return err
	}
	if result.UpdateAvailable && result.LatestVersion != nil {
		if !check {
			_, err := fmt.Fprintf(out, language.Text("output.updated_pixiv_to"), *result.LatestVersion)
			return err
		}
		_, err := fmt.Fprintf(out, language.Text("output.update_available_run_pixiv_update"), result.CurrentVersion, *result.LatestVersion)
		return err
	}
	_, err := fmt.Fprintf(out, language.Text("output.pixiv_is_up_to_date"), result.CurrentVersion)
	return err
}
