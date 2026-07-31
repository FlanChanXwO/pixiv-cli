package cli

import (
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/loginhelper"
	"github.com/spf13/cobra"
)

const internalURLCallbackCommand = loginhelper.CallbackCommand
const internalURLHandlerInstallCommand = "_install-handler"

// clearRemoteLoginHandoffForHandler 是狭窄的测试注入点，覆盖 browser 启动失败
// 后本地一次性状态也无法清理的错误边界。
var clearRemoteLoginHandoffForHandler = loginhelper.ClearRemoteLoginHandoff

// newAccountURLCallbackCommand 仅供各桌面系统协议关联调用。它不属于公开 CLI
// 契约：活跃 loopback bridge 优先，随后才转发精确白名单 callback 到本次
// remote handoff；其他 pixiv:// URL 会定向交回此前 handler。
func (a app) newAccountURLCallbackCommand() *cobra.Command {
	return &cobra.Command{
		Use:    internalURLCallbackCommand + " URL",
		Hidden: true,
		Args:   requireExactArgs(1, "pixiv auth "+internalURLCallbackCommand+" URL"),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := loginhelper.HandleCallback(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			browserOpener, _, _ := currentLoginHooks()
			if result.RemoteLoginStart != nil {
				loginURL, err := loginhelper.StartRemoteLogin(cmd.Context(), *result.RemoteLoginStart)
				if err != nil {
					return err
				}
				if err := browserOpener(loginURL); err != nil {
					if clearErr := clearRemoteLoginHandoffForHandler(*result.RemoteLoginStart); clearErr != nil {
						return errors.New("could not clear remote Pixiv login handoff after browser launch failed")
					}
					return errors.New("could not open Pixiv authorization page")
				}
				return nil
			}
			if result.LocalRelayURL != "" {
				if err := browserOpener(result.LocalRelayURL); err != nil {
					// browser.OpenURL 的错误可能含本地路径或完整 URL；内部 handler 的
					// stderr 是协议调用方可见边界，只保留稳定类别。
					return errors.New("could not open Pixiv login callback bridge")
				}
				return nil
			}
			if result.RemoteCallback == nil {
				return nil
			}
			if err := browserOpener(result.RemoteCallback.ResultURL); err != nil {
				result.RemoteCallback.Abort()
				return errors.New("could not open Pixiv login result page")
			}
			if err := result.RemoteCallback.Complete(); err != nil {
				return err
			}
			return nil
		},
	}
}

// newAccountURLHandlerInstallCommand 仅给官方安装器与 Homebrew post_install 使用。
// 系统集成失败不应回滚已验证的 binary；它会明确警告，而后续正常 browser login
// 仍会再次尝试初始化 handler。
func (a app) newAccountURLHandlerInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:    internalURLHandlerInstallCommand,
		Hidden: true,
		Args:   requireExactArgs(0, "pixiv auth "+internalURLHandlerInstallCommand),
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _, ensure := currentLoginHooks()
			if err := ensure(cmd.Context()); err != nil {
				fmt.Fprintf(a.errOut, "warning: persistent pixiv:// callback handler was not initialized: %v\n", err)
			}
			return nil
		},
	}
}
