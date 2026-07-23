package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

const configMissingPlaceholder = "<unset>"

const configKeyHelp = "download_path, filename_template, https_proxy, log_level, login_open_browser, login_timeout, login_use_after_login, output_json, update_check_enabled, web_fallback_enabled"

func (a app) newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage global Pixiv CLI settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		a.newConfigPathCommand(),
		a.newConfigGetCommand(),
		a.newConfigSetCommand(),
		a.newConfigUnsetCommand(),
	)
	return cmd
}

func (a app) newConfigPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config.toml path",
		Args:  requireExactArgs(0, "pixiv config path"),
		RunE: func(cmd *cobra.Command, args []string) error {
			services := a.services()
			path, err := services.Config.Path()
			if err != nil {
				return err
			}
			fmt.Fprintln(a.out, path)
			return nil
		},
	}
}

func (a app) newConfigGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get KEY",
		Short: "Print one effective config value",
		Long:  "Print one effective config value. KEY must be one of: " + configKeyHelp,
		Args:  requireExactArgs(1, "pixiv config get KEY"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.configGet(args[0])
		},
	}
}

func (a app) configGet(alias string) error {
	services := a.services()
	value, err := services.Config.Get(alias)
	if err != nil {
		return err
	}
	if !value.HasValue {
		fmt.Fprintln(a.out, configMissingPlaceholder)
		return errors.New("config value is unset")
	}
	fmt.Fprintln(a.out, value.Text)
	return nil
}

func (a app) newConfigSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set KEY VALUE",
		Short: "Set one config value in config.toml",
		Long:  "Set one config value in config.toml. KEY must be one of: " + configKeyHelp,
		Args:  requireExactArgs(2, "pixiv config set KEY VALUE"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.configSet(args[0], args[1])
		},
	}
}

func (a app) configSet(alias, raw string) error {
	services := a.services()
	result, err := services.Config.Set(alias, raw)
	if err != nil {
		return err
	}
	if result.HasOverride {
		a.writeConfigOverrideNote(alias, result.EnvOverride)
	}
	fmt.Fprintf(a.out, "%s updated\n", alias)
	return nil
}

func (a app) writeConfigOverrideNote(alias, envOverride string) {
	if alias == "https_proxy" {
		// 代理 URL 可能携带 userinfo、路径或 query，提示覆盖来源即可，不能回显其值。
		fmt.Fprintf(a.errOut, "note: %s is currently overridden by environment; effective value remains controlled by environment\n", alias)
		return
	}
	fmt.Fprintf(a.errOut, "note: %s is currently overridden by environment and effective value remains %q\n", alias, envOverride)
}

func (a app) newConfigUnsetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unset KEY",
		Short: "Remove one config value from config.toml",
		Long:  "Remove one config value from config.toml. KEY must be one of: " + configKeyHelp,
		Args:  requireExactArgs(1, "pixiv config unset KEY"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.configUnset(args[0])
		},
	}
}

func (a app) configUnset(alias string) error {
	services := a.services()
	result, err := services.Config.Unset(alias)
	if err != nil {
		return err
	}
	if result.HasOverride {
		a.writeConfigOverrideNote(alias, result.EnvOverride)
	}
	fmt.Fprintf(a.out, "%s removed\n", alias)
	return nil
}
