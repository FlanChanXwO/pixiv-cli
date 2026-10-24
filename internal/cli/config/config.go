// Package config 注册 config 命令。
package config

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/spf13/cobra"
)

const configMissingPlaceholder = "<unset>"

func configKeyHelp() string {
	keys := settings.CLISettingAliases()
	names := make([]string, len(keys))
	for index, key := range keys {
		names[index] = string(key)
	}
	return strings.Join(names, ", ")
}

func isCLIConfigAlias(key settings.Key) bool {
	spec, ok := settings.SettingSpecByAlias(key)
	return ok && spec.CLIManaged
}

func invalidCLIConfigKey(key settings.Key) error {
	return fmt.Errorf("unknown config key %q. valid keys: %s", key, configKeyHelp())
}

type Host interface {
	Input() io.Reader
	Output() io.Writer
	ErrorOutput() io.Writer
	UsageError(error) error
	RequireExactArgs(int, string) cobra.PositionalArgs
	ConfigService() settings.Store
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
	input.Bind(cmd, input.InputSpec{Codec: input.NoInput, MinArgs: 0, MaxArgs: 0, Reader: host.Input(), UsageError: host.UsageError})
	command.Bind(cmd, command.Execution{})
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
	input.Bind(cmd, input.InputSpec{Codec: input.NoInput, MinArgs: 0, MaxArgs: 0, Reader: host.Input(), UsageError: host.UsageError})
	command.Bind(cmd, command.ConfigPath())
	return cmd
}

func newGetCommand(host Host) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get KEY",
		Short: "Print one effective config value",
		Long:  "Print one effective config value. KEY must be one of: " + configKeyHelp(),
		Args:  host.RequireExactArgs(1, "pixiv config get KEY"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return get(host, settings.Key(args[0]))
		},
	}
	input.Bind(cmd, input.InputSpec{Codec: input.TextValue, MinArgs: 1, MaxArgs: 1, FillPosition: 0, Reader: host.Input(), UsageError: host.UsageError})
	command.Bind(cmd, command.Normal())
	return cmd
}

func get(host Host, key settings.Key) error {
	if !isCLIConfigAlias(key) {
		return invalidCLIConfigKey(key)
	}
	value, err := host.ConfigService().Get(key)
	if err != nil {
		return err
	}
	if spec, ok := settings.SettingSpecByAlias(key); ok && spec.Sensitive {
		_, err := fmt.Fprintln(host.Output(), "<redacted>")
		return err
	}
	if !value.HasValue {
		_, _ = fmt.Fprintln(host.Output(), configMissingPlaceholder)
		return errors.New("config value is unset")
	}
	_, err = fmt.Fprintln(host.Output(), settings.PublicSettingText(key, value.Text))
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
			key := settings.Key(args[0])
			if !isCLIConfigAlias(key) {
				return invalidCLIConfigKey(key)
			}
			if settings.IsSensitiveSetting(key) && len(args) == 2 {
				return errors.New("sensitive config values must be provided through non-TTY stdin")
			}
			if settings.IsSensitiveSetting(key) {
				if input.IsTTY(host.Input()) {
					return errors.New("sensitive config values require non-TTY stdin")
				}
				return nil
			}
			if len(args) != 2 {
				return errors.New("VALUE is required for this config key")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := settings.Key(args[0])
			if settings.IsSensitiveSetting(key) {
				raw, err := input.ReadValue(host.Input())
				if err != nil {
					return fmt.Errorf("read sensitive config value from stdin: %w", err)
				}
				return set(host, key, raw)
			}
			return set(host, key, args[1])
		},
	}
	input.Bind(cmd, input.InputSpec{
		Codec: input.TextValue, MinArgs: 1, MaxArgs: 2, FillPosition: 1,
		Reader: host.Input(), UsageError: host.UsageError,
		Enabled: func(_ *cobra.Command, args []string) bool {
			return len(args) == 0 || !settings.IsSensitiveSetting(settings.Key(args[0]))
		},
	})
	command.Bind(cmd, command.Normal())
	return cmd
}

func set(host Host, key settings.Key, raw string) error {
	if !isCLIConfigAlias(key) {
		return invalidCLIConfigKey(key)
	}
	result, err := host.ConfigService().Set(key, raw)
	if err != nil {
		return err
	}
	if result.HasOverride {
		writeOverrideNote(host.ErrorOutput(), key, result.EnvOverride)
	}
	_, err = fmt.Fprintf(host.Output(), "%s updated\n", key)
	return err
}

func newUnsetCommand(host Host) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset KEY",
		Short: "Remove one config value from config.toml",
		Long:  "Remove one config value from config.toml. KEY must be one of: " + configKeyHelp(),
		Args:  host.RequireExactArgs(1, "pixiv config unset KEY"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return unset(host, settings.Key(args[0]))
		},
	}
	input.Bind(cmd, input.InputSpec{Codec: input.TextValue, MinArgs: 1, MaxArgs: 1, FillPosition: 0, Reader: host.Input(), UsageError: host.UsageError})
	command.Bind(cmd, command.Normal())
	return cmd
}

func unset(host Host, key settings.Key) error {
	if !isCLIConfigAlias(key) {
		return invalidCLIConfigKey(key)
	}
	result, err := host.ConfigService().Unset(key)
	if err != nil {
		return err
	}
	if result.HasOverride {
		writeOverrideNote(host.ErrorOutput(), key, result.EnvOverride)
	}
	_, err = fmt.Fprintf(host.Output(), "%s removed\n", key)
	return err
}

func writeOverrideNote(out io.Writer, key settings.Key, envOverride string) {
	if key == settings.KeyHTTPSProxy || settings.IsSensitiveSetting(key) {
		_, _ = fmt.Fprintf(out, "note: %s is currently overridden by environment; effective value remains controlled by environment\n", key)
		return
	}
	_, _ = fmt.Fprintf(out, "note: %s is currently overridden by environment and effective value remains %q\n", key, envOverride)
}
