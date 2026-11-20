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

// RunAutomaticCheck 在业务命令成功后展示稳定版更新提示。所有错误只写
// stderr，不改变已成功命令的退出码，也不进入 stdout/MCP JSON-RPC。
func RunAutomaticCheck(cmd *cobra.Command, errOut io.Writer, language i18n.Locale, loadRuntime func() (config.Config, error)) {
	if !shouldCheck(cmd) {
		return
	}
	runtimeConfig, err := loadRuntime()
	if err != nil {
		warning(errOut, language, language.Text("update.load_config"), err)
		return
	}
	if !runtimeConfig.Update.CheckEnabled {
		return
	}
	proxy, err := automaticProxy(cmd, runtimeConfig)
	if err != nil {
		warning(errOut, language, language.Text("update.read_proxy"), err)
		return
	}
	checker, err := NewAutomaticChecker(proxy)
	if err != nil {
		warning(errOut, language, language.Text("update.create_checker"), err)
		return
	}
	notice, err := checker.Check(cmd.Context(), selfupdate.AutomaticUpdateRequest{BuildInfo: version.Current()})
	if err != nil {
		warning(errOut, language, language.Text("update.check"), err)
		return
	}
	if notice == nil {
		return
	}
	_, _ = fmt.Fprintf(errOut, language.Text("output.update_available_run"), notice.CurrentVersion, notice.LatestVersion, notice.UpdateCommand)
}

func warning(errOut io.Writer, language i18n.Locale, action string, err error) {
	_, _ = fmt.Fprintf(errOut, language.Text("output.warning_441"), action, language.ErrorText(err))
}

func shouldCheck(cmd *cobra.Command) bool {
	if version.Current().IsDevelopment() || cmd.Root() == cmd {
		return false
	}
	if cmd.CommandPath() == "pixiv auth export" {
		return false
	}
	if environment.IsOffline(cmd) {
		return false
	}
	if helpFlag := cmd.Flags().Lookup("help"); helpFlag != nil && helpFlag.Changed {
		return false
	}
	if cmd.HasSubCommands() {
		return false
	}
	for current := cmd; current != nil; current = current.Parent() {
		switch current.Name() {
		case "help", "mcp", "update", "_callback":
			return false
		}
	}
	return true
}

func automaticProxy(cmd *cobra.Command, runtimeConfig config.Config) (string, error) {
	proxyFlag := cmd.Flags().Lookup("proxy")
	noProxyFlag := cmd.Flags().Lookup("no-proxy")
	if proxyFlag == nil && noProxyFlag == nil {
		return runtimeConfig.Network.HTTPSProxy, nil
	}
	if proxyFlag != nil && proxyFlag.Changed && noProxyFlag != nil && noProxyFlag.Changed {
		return "", i18n.Errorf("error.use_either_proxy_or_no_proxy_not_both")
	}
	if noProxyFlag != nil && noProxyFlag.Changed {
		return "", nil
	}
	if proxyFlag != nil && proxyFlag.Changed {
		proxy, err := cmd.Flags().GetString("proxy")
		if err != nil {
			return "", i18n.Errorf("error.read_proxy", err)
		}
		return proxy, nil
	}
	return runtimeConfig.Network.HTTPSProxy, nil
}
