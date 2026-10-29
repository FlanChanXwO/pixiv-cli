// Package config 提供 pixiv config 命令：显示配置路径、读取、设置和取消设置。
//
// 包的组成：[New] 组装命令树。
// 设置存储与值解析归 [github.com/FlanChanXwO/pixiv-cli/internal/config/settings]。
package config

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
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

// New 构造 config 命令组；store 是 config.toml 的唯一读写入口。
func New(env environment.Env, store settings.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage global Pixiv CLI settings",
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newPathCommand(env, store), newGetCommand(env, store), newSetCommand(env, store), newUnsetCommand(env, store))
	environment.BindInput(cmd, environment.InputSpec{Codec: environment.NoInput, MinArgs: 0, MaxArgs: 0, Reader: env.In, UsageError: env.WrapUsage})
	environment.Bind(cmd, environment.Execution{})
	return cmd
}

func newPathCommand(env environment.Env, store settings.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Print the config.toml path",
		Args:  env.ExactArgs(0, "pixiv config path"),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := store.Path()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(env.Out, path)
			return err
		},
	}
	environment.BindInput(cmd, environment.InputSpec{Codec: environment.NoInput, MinArgs: 0, MaxArgs: 0, Reader: env.In, UsageError: env.WrapUsage})
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func newGetCommand(env environment.Env, store settings.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get KEY",
		Short: "Print one effective config value",
		Long:  "Print one effective config value. KEY must be one of: " + configKeyHelp(),
		Args:  env.ExactArgs(1, "pixiv config get KEY"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return get(env, store, settings.Key(args[0]))
		},
	}
	environment.BindInput(cmd, environment.InputSpec{Codec: environment.TextValue, MinArgs: 1, MaxArgs: 1, FillPosition: 0, Reader: env.In, UsageError: env.WrapUsage})
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func get(env environment.Env, store settings.Store, key settings.Key) error {
	if !isCLIConfigAlias(key) {
		return invalidCLIConfigKey(key)
	}
	value, err := store.Get(key)
	if err != nil {
		return err
	}
	if settings.IsSensitiveSetting(key) {
		_, err := fmt.Fprintln(env.Out, "<redacted>")
		return err
	}
	if !value.HasValue {
		_, _ = fmt.Fprintln(env.Out, configMissingPlaceholder)
		return errors.New("config value is unset")
	}
	_, err = fmt.Fprintln(env.Out, settings.PublicSettingText(key, value.Text))
	return err
}

func newSetCommand(env environment.Env, store settings.Store) *cobra.Command {
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
				if environment.IsTTY(env.In) {
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
				raw, err := environment.ReadValue(env.In)
				if err != nil {
					return fmt.Errorf("read sensitive config value from stdin: %w", err)
				}
				return set(env, store, key, raw)
			}
			return set(env, store, key, args[1])
		},
	}
	environment.BindInput(cmd, environment.InputSpec{
		Codec: environment.TextValue, MinArgs: 1, MaxArgs: 2, FillPosition: 1,
		Reader: env.In, UsageError: env.WrapUsage,
		Enabled: func(_ *cobra.Command, args []string) bool {
			return len(args) == 0 || !settings.IsSensitiveSetting(settings.Key(args[0]))
		},
	})
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func set(env environment.Env, store settings.Store, key settings.Key, raw string) error {
	result, err := store.Set(key, raw)
	if err != nil {
		return err
	}
	if result.HasOverride {
		writeOverrideNote(env.Err, key, result.EnvOverride)
	}
	_, err = fmt.Fprintf(env.Out, "%s updated\n", key)
	return err
}

func newUnsetCommand(env environment.Env, store settings.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset KEY",
		Short: "Remove one config value from config.toml",
		Long:  "Remove one config value from config.toml. KEY must be one of: " + configKeyHelp(),
		Args:  env.ExactArgs(1, "pixiv config unset KEY"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return unset(env, store, settings.Key(args[0]))
		},
	}
	environment.BindInput(cmd, environment.InputSpec{Codec: environment.TextValue, MinArgs: 1, MaxArgs: 1, FillPosition: 0, Reader: env.In, UsageError: env.WrapUsage})
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func unset(env environment.Env, store settings.Store, key settings.Key) error {
	if !isCLIConfigAlias(key) {
		return invalidCLIConfigKey(key)
	}
	result, err := store.Unset(key)
	if err != nil {
		return err
	}
	if result.HasOverride {
		writeOverrideNote(env.Err, key, result.EnvOverride)
	}
	_, err = fmt.Fprintf(env.Out, "%s removed\n", key)
	return err
}

func writeOverrideNote(out io.Writer, key settings.Key, envOverride string) {
	if key == settings.KeyHTTPSProxy || settings.IsSensitiveSetting(key) {
		_, _ = fmt.Fprintf(out, "note: %s is currently overridden by environment; effective value remains controlled by environment\n", key)
		return
	}
	_, _ = fmt.Fprintf(out, "note: %s is currently overridden by environment and effective value remains %q\n", key, envOverride)
}
