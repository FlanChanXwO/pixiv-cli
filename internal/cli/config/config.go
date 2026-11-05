// Package config 提供配置路径查询与 TOML 叶子键的读写命令。
//
// [New] 通过配置存储构造命令树。
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/spf13/cobra"
)

// New 构造 config 命令组，API key 与其他字符串使用相同的输入输出规则。
func New(env environment.Env, store config.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use: "config", Short: "Manage global Pixiv CLI settings",
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	path := &cobra.Command{
		Use: "path", Short: "Print the config.toml path",
		Args: env.ExactArgs(0, "pixiv config path"),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := store.Path()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(env.Out, path)
			return err
		},
	}
	environment.BindInput(path, environment.InputSpec{Codec: environment.NoInput, MinArgs: 0, MaxArgs: 0, Reader: env.In, UsageError: env.WrapUsage})
	environment.Bind(path, environment.Normal())
	cmd.AddCommand(path)
	for _, operation := range []string{"get", "set", "unset"} {
		count := 1
		usage := operation + " KEY"
		if operation == "set" {
			count = 2
			usage += " VALUE"
		}
		leaf := &cobra.Command{
			Use: usage, Short: strings.ToUpper(operation[:1]) + operation[1:] + " one config value",
			Long: "Use a TOML path. Available keys: " + strings.Join(config.Keys(), ", "),
			Args: env.ExactArgs(count, "pixiv config "+usage),
			RunE: func(cmd *cobra.Command, args []string) error {
				key := args[0]
				switch operation {
				case "get":
					value, present, err := store.Get(key)
					if err != nil {
						return err
					}
					if !present {
						_, writeErr := fmt.Fprintln(env.Out, "<unset>")
						return errors.Join(errors.New("config value is unset"), writeErr)
					}
					_, err = fmt.Fprintln(env.Out, value)
					return err
				case "set":
					if err := store.Set(key, args[1]); err != nil {
						return err
					}
				case "unset":
					if err := store.Unset(key); err != nil {
						return err
					}
				}
				if config.EnvironmentOverride(key) {
					if _, err := fmt.Fprintf(env.Err, "note: %s is currently overridden by environment\n", key); err != nil {
						return err
					}
				}
				action := "updated"
				if operation == "unset" {
					action = "removed"
				}
				_, err := fmt.Fprintf(env.Out, "%s %s\n", key, action)
				return err
			},
		}
		environment.BindInput(leaf, environment.InputSpec{Codec: environment.TextValue, MinArgs: 1, MaxArgs: count, FillPosition: count - 1, Reader: env.In, UsageError: env.WrapUsage})
		environment.Bind(leaf, environment.Normal())
		cmd.AddCommand(leaf)
	}
	environment.BindInput(cmd, environment.InputSpec{Codec: environment.NoInput, MinArgs: 0, MaxArgs: 0, Reader: env.In, UsageError: env.WrapUsage})
	environment.Bind(cmd, environment.Execution{})
	return cmd
}
