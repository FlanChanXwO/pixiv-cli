package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/config"
	"github.com/spf13/cobra"
)

const configMissingPlaceholder = "<unset>"

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
			path, err := config.ConfigFilePath()
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
		Args:  requireExactArgs(1, "pixiv config get KEY"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.configGet(args[0])
		},
	}
}

func (a app) configGet(alias string) error {
	settings, err := config.LoadSettingsState()
	if err != nil {
		return err
	}
	value, err := settings.Effective(alias)
	if err != nil {
		return fmt.Errorf("%w. valid keys: %s", err, strings.Join(config.ValidSettingAliases(), ", "))
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
		Args:  requireExactArgs(2, "pixiv config set KEY VALUE"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.configSet(args[0], args[1])
		},
	}
}

func (a app) configSet(alias, raw string) error {
	spec, ok := config.SettingSpecByAlias(alias)
	if !ok {
		return fmt.Errorf("unknown config key %q. valid keys: %s", alias, strings.Join(config.ValidSettingAliases(), ", "))
	}
	_, value, err := config.ParseSettingInput(alias, raw)
	if err != nil {
		return err
	}
	path, err := config.ConfigFilePath()
	if err != nil {
		return err
	}
	if err := config.SetConfigValue(path, alias, value); err != nil {
		return err
	}
	if envRaw, ok := config.EnvValue(spec); ok {
		fmt.Fprintf(a.errOut, "note: %s is currently overridden by environment and effective value remains %q\n", alias, envRaw)
	}
	fmt.Fprintf(a.out, "%s updated\n", alias)
	return nil
}

func (a app) newConfigUnsetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unset KEY",
		Short: "Remove one config value from config.toml",
		Args:  requireExactArgs(1, "pixiv config unset KEY"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.configUnset(args[0])
		},
	}
}

func (a app) configUnset(alias string) error {
	spec, ok := config.SettingSpecByAlias(alias)
	if !ok {
		return fmt.Errorf("unknown config key %q. valid keys: %s", alias, strings.Join(config.ValidSettingAliases(), ", "))
	}
	path, err := config.ConfigFilePath()
	if err != nil {
		return err
	}
	_, err = config.UnsetConfigValue(path, alias)
	if err != nil {
		return err
	}
	if envRaw, ok := config.EnvValue(spec); ok {
		fmt.Fprintf(a.errOut, "note: %s is currently overridden by environment and effective value remains %q\n", alias, envRaw)
	}
	fmt.Fprintf(a.out, "%s removed\n", alias)
	return nil
}
