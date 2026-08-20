// Package config 注册 config 命令。
package config

import (
	"errors"
	"fmt"
	"io"
	"strings"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/spf13/cobra"
)

const configMissingPlaceholder = "<unset>"

func configKeyHelp() string {
	return strings.Join(configapp.CLISettingAliases(), ", ")
}

func isCLIConfigAlias(alias string) bool {
	spec, ok := configapp.SettingSpecByAlias(alias)
	return ok && !spec.Removed && spec.CLIManaged
}

func isRemovedConfigAlias(alias string) bool {
	spec, ok := configapp.SettingSpecByAlias(alias)
	return ok && spec.Removed
}

func invalidCLIConfigKey(alias string) error {
	return fmt.Errorf("unknown config key %q. valid keys: %s", alias, configKeyHelp())
}

type Host interface {
	Input() io.Reader
	Output() io.Writer
	ErrorOutput() io.Writer
	UsageError(error) error
	RequireExactArgs(int, string) cobra.PositionalArgs
	ConfigService() configapp.Store
	ConfigPath() (string, error)
}

func Register(root *cobra.Command, host Host) {
	root.AddCommand(NewCommand(host))
}

// NewCommand 只负责将 config 命令注册到同目录实现。
func NewCommand(host Host) *cobra.Command {
	return newConfigCommand(host)
}

func newConfigCommand(host Host) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage global Pixiv CLI settings",
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newPathCommand(host), newGetCommand(host), newSetCommand(host), newUnsetCommand(host))
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.NoInput, MinArgs: 0, MaxArgs: 0, Reader: host.Input(), UsageError: host.UsageError})
	requirements.Bind(cmd, requirements.Execution{})
	return cmd
}

func newPathCommand(host Host) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Print the config.toml path",
		Args:  host.RequireExactArgs(0, "pixiv config path"),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := host.ConfigPath()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(host.Output(), path)
			return err
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.NoInput, MinArgs: 0, MaxArgs: 0, Reader: host.Input(), UsageError: host.UsageError})
	requirements.Bind(cmd, requirements.ConfigPath())
	return cmd
}

func newGetCommand(host Host) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get KEY",
		Short: "Print one effective config value",
		Long:  "Print one effective config value. KEY must be one of: " + configKeyHelp(),
		Args:  host.RequireExactArgs(1, "pixiv config get KEY"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return get(host, args[0])
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.TextValue, MinArgs: 1, MaxArgs: 1, FillPosition: 0, Reader: host.Input(), UsageError: host.UsageError})
	requirements.Bind(cmd, requirements.Normal())
	return cmd
}

func get(host Host, alias string) error {
	if isRemovedConfigAlias(alias) {
		return configapp.RemovedSettingError(alias)
	}
	if !isCLIConfigAlias(alias) {
		return invalidCLIConfigKey(alias)
	}
	value, err := host.ConfigService().Get(alias)
	if err != nil {
		return err
	}
	if !value.HasValue {
		_, _ = fmt.Fprintln(host.Output(), configMissingPlaceholder)
		return errors.New("config value is unset")
	}
	if spec, ok := configapp.SettingSpecByAlias(alias); ok && spec.Sensitive {
		_, err := fmt.Fprintln(host.Output(), "<redacted>")
		return err
	}
	_, err = fmt.Fprintln(host.Output(), configapp.PublicSettingText(alias, value.Text))
	return err
}

func newSetCommand(host Host) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set KEY [VALUE]",
		Short: "Set one config value in config.toml",
		Long:  "Set one config value in config.toml. KEY must be one of: " + configKeyHelp(),
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return errors.New("usage: pixiv config set KEY [VALUE]")
			}
			if isRemovedConfigAlias(args[0]) {
				return configapp.RemovedSettingError(args[0])
			}
			if !isCLIConfigAlias(args[0]) {
				return invalidCLIConfigKey(args[0])
			}
			if len(args) != 2 {
				return errors.New("VALUE is required for this config key")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return set(host, args[0], args[1])
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.TextValue, MinArgs: 1, MaxArgs: 2, FillPosition: 1, Reader: host.Input(), UsageError: host.UsageError})
	requirements.Bind(cmd, requirements.Normal())
	return cmd
}

func set(host Host, alias, raw string) error {
	if isRemovedConfigAlias(alias) {
		return configapp.RemovedSettingError(alias)
	}
	if !isCLIConfigAlias(alias) {
		return invalidCLIConfigKey(alias)
	}
	result, err := host.ConfigService().Set(alias, raw)
	if err != nil {
		return err
	}
	if result.HasOverride {
		writeOverrideNote(host.ErrorOutput(), alias, result.EnvOverride)
	}
	_, err = fmt.Fprintf(host.Output(), "%s updated\n", alias)
	return err
}

func newUnsetCommand(host Host) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset KEY",
		Short: "Remove one config value from config.toml",
		Long:  "Remove one config value from config.toml. KEY must be one of: " + configKeyHelp(),
		Args:  host.RequireExactArgs(1, "pixiv config unset KEY"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return unset(host, args[0])
		},
	}
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.TextValue, MinArgs: 1, MaxArgs: 1, FillPosition: 0, Reader: host.Input(), UsageError: host.UsageError})
	requirements.Bind(cmd, requirements.Normal())
	return cmd
}

func unset(host Host, alias string) error {
	if !isCLIConfigAlias(alias) && !isRemovedConfigAlias(alias) {
		return invalidCLIConfigKey(alias)
	}
	result, err := host.ConfigService().Unset(alias)
	if err != nil {
		return err
	}
	if result.HasOverride {
		writeOverrideNote(host.ErrorOutput(), alias, result.EnvOverride)
	}
	_, err = fmt.Fprintf(host.Output(), "%s removed\n", alias)
	return err
}

func writeOverrideNote(out io.Writer, alias, envOverride string) {
	if alias == "https_proxy" {
		_, _ = fmt.Fprintf(out, "note: %s is currently overridden by environment; effective value remains controlled by environment\n", alias)
		return
	}
	_, _ = fmt.Fprintf(out, "note: %s is currently overridden by environment and effective value remains %q\n", alias, envOverride)
}
