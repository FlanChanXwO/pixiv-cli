package cli

import (
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/loginhelper"
	"github.com/spf13/cobra"
)

const internalURLCallbackCommand = loginhelper.CallbackCommand

// newAccountURLCallbackCommand 仅供 Linux desktop entry 与 Windows 协议关联调用。
// 它不属于公开 CLI 契约，也不读取账号、配置或浏览器状态；系统传入的 pixiv://
// URL 只会被转为当前登录尝试的 loopback bridge fragment。
func (a app) newAccountURLCallbackCommand() *cobra.Command {
	return &cobra.Command{
		Use:    internalURLCallbackCommand + " URL",
		Hidden: true,
		Args:   requireExactArgs(1, "pixiv auth "+internalURLCallbackCommand+" URL"),
		RunE: func(_ *cobra.Command, args []string) error {
			relayURL, err := loginhelper.CallbackRelayURL(args[0])
			if err != nil {
				return err
			}
			browserOpener, _ := currentLoginHooks()
			if err := browserOpener(relayURL); err != nil {
				// browser.OpenURL 的错误可能含本地路径或完整 URL；内部 handler 的
				// stderr 是协议调用方可见边界，只保留稳定类别。
				return errors.New("could not open Pixiv login callback bridge")
			}
			return nil
		},
	}
}
