package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/loginhelper"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/spf13/cobra"
)

const configMissingPlaceholder = "<unset>"

const configKeyHelp = "download_path, filename_template, https_proxy"

var cliConfigAliases = map[string]struct{}{
	"download_path":     {},
	"filename_template": {},
	"https_proxy":       {},
}

func isCLIConfigAlias(alias string) bool {
	_, ok := cliConfigAliases[alias]
	return ok
}

func invalidCLIConfigKey(alias string) error {
	return fmt.Errorf("unknown config key %q. valid keys: %s", alias, configKeyHelp)
}

// ensurePersistentURLHandler 为 config 命令保留窄注入点：测试不能为了验证
// 配置错误路径而修改真实桌面系统的 pixiv:// association。
var (
	ensurePersistentURLHandler  = loginhelper.EnsurePersistent
	disablePersistentURLHandler = loginhelper.DisablePersistent
)

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
	if !isCLIConfigAlias(alias) {
		return invalidCLIConfigKey(alias)
	}
	services := a.services()
	value, err := services.Config.Get(alias)
	if err != nil {
		return err
	}
	if !value.HasValue {
		fmt.Fprintln(a.out, configMissingPlaceholder)
		return errors.New("config value is unset")
	}
	if spec, ok := config.SettingSpecByAlias(alias); ok && spec.Sensitive {
		// relay secret 是 bearer credential。即使配置文件是私有文件，也不能经由
		// config get 形成 stdout、terminal scrollback 或自动化日志泄露。
		fmt.Fprintln(a.out, "<redacted>")
		return nil
	}
	fmt.Fprintln(a.out, config.PublicSettingText(alias, value.Text))
	return nil
}

func (a app) newConfigSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set KEY [VALUE]",
		Short: "Set one config value in config.toml",
		Long:  "Set one config value in config.toml. KEY must be one of: " + configKeyHelp,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return errors.New("usage: pixiv config set KEY [VALUE]")
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
			return a.configSet(args[0], args[1])
		},
	}
}

func (a app) configSet(alias, raw string) error {
	if !isCLIConfigAlias(alias) {
		return invalidCLIConfigKey(alias)
	}
	services := a.services()
	result, err := services.Config.Set(alias, raw)
	if err != nil {
		return err
	}
	if result.HasOverride {
		a.writeConfigOverrideNote(alias, result.EnvOverride)
	}
	if relayConfigUsesHTTP(alias, raw) {
		// HTTP relay 会在 client 与 server 之间传输 callback 和 bearer secret；
		// 写配置时即告知风险，不能只等到某次 server login 才暴露。即使后续
		// handler 初始化失败，刚刚写入的 HTTP 配置也仍需被用户明确看见。
		fmt.Fprintln(a.errOut, "warning: remote Pixiv login relay uses HTTP; the callback and relay secret can be observed or modified by the network.")
	}
	if alias == "login_relay_target_url" || alias == "login_relay_secret" {
		// 两项齐全才接管系统 handler；允许用户先后设置它们，避免半配置在
		// Windows/Linux/macOS 留下错误的默认 URL association。
		_, relayErr := loginhelper.ConfiguredRelayTarget()
		if relayErr == nil {
			if err := ensurePersistentURLHandler(context.Background()); err != nil {
				return fmt.Errorf("enable persistent Pixiv callback handler: %w", err)
			}
		} else if !errors.Is(relayErr, loginhelper.ErrNoConfiguredRelay) && !errors.Is(relayErr, loginhelper.ErrIncompleteRelayConfig) {
			return relayErr
		}
	}
	fmt.Fprintf(a.out, "%s updated\n", alias)
	return nil
}

func relayConfigUsesHTTP(alias, raw string) bool {
	if alias != "login_relay_public_url" && alias != "login_relay_target_url" {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(parsed.Scheme, "http")
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
	if !isCLIConfigAlias(alias) {
		return invalidCLIConfigKey(alias)
	}
	if alias == "login_relay_target_url" {
		// 先恢复系统关联；若恢复失败，保留配置让用户可重试，不能假装已经清理。
		if err := disablePersistentURLHandler(context.Background()); err != nil {
			return fmt.Errorf("disable persistent Pixiv callback handler: %w", err)
		}
	}
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
