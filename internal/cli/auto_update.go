package cli

import (
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/bootstrap"
	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/spf13/cobra"
)

var (
	loadAutomaticUpdateRuntimeConfig = bootstrap.LoadRuntimeConfig
	newCLIAutomaticUpdateChecker     = bootstrap.NewAutomaticUpdateChecker
)

// checkAutomaticUpdate 在业务命令成功后尽力展示稳定版更新提示。
// 自动检查不能改变业务退出码；配置、网络或来源识别失败只写 stderr 供用户排查。
func (a app) checkAutomaticUpdate(cmd *cobra.Command) {
	if !shouldCheckAutomaticUpdate(cmd) {
		return
	}
	runtimeConfig, err := loadAutomaticUpdateRuntimeConfig()
	if err != nil {
		a.automaticUpdateWarning("load automatic update configuration", err)
		return
	}
	if !runtimeConfig.UpdateCheckEnabled {
		return
	}
	proxy, err := automaticUpdateProxy(cmd, runtimeConfig)
	if err != nil {
		a.automaticUpdateWarning("read automatic update proxy override", err)
		return
	}
	checker, err := newCLIAutomaticUpdateChecker(proxy)
	if err != nil {
		a.automaticUpdateWarning("create automatic update checker", err)
		return
	}
	notice, err := checker.Check(cmd.Context(), update.AutomaticUpdateRequest{BuildInfo: buildinfo.Current()})
	if err != nil {
		a.automaticUpdateWarning("check for updates", err)
		return
	}
	if notice == nil {
		return
	}
	// 标准输出可能是 JSON 或 MCP 协议数据；用户可读提示永远只能去 stderr。
	fmt.Fprintf(a.errOut, "update available: %s -> %s\nrun: %s\n", notice.CurrentVersion, notice.LatestVersion, notice.UpdateCommand)
}

func (a app) automaticUpdateWarning(action string, err error) {
	fmt.Fprintf(a.errOut, "warning: %s: %v\n", action, err)
}

func shouldCheckAutomaticUpdate(cmd *cobra.Command) bool {
	if buildinfo.Current().IsDevelopment() || cmd == nil || cmd.Root() == cmd {
		return false
	}
	if helpFlag := cmd.Flags().Lookup("help"); helpFlag != nil && helpFlag.Changed {
		return false
	}
	// 有子命令但自身只展示帮助的命令不属于普通业务成功路径。
	if cmd.HasSubCommands() {
		return false
	}
	for current := cmd; current != nil; current = current.Parent() {
		switch current.Name() {
		case "help", "mcp", "version", "update":
			return false
		}
	}
	return true
}

// automaticUpdateProxy 只读取当前已声明的 --proxy/--no-proxy flag。
// 这与具体业务命令无关，同时不会把不存在的 flag 误当成更新器选项。
func automaticUpdateProxy(cmd *cobra.Command, runtimeConfig config.RuntimeConfig) (string, error) {
	proxyFlag := cmd.Flags().Lookup("proxy")
	noProxyFlag := cmd.Flags().Lookup("no-proxy")
	if proxyFlag == nil && noProxyFlag == nil {
		return runtimeConfig.HTTPSProxy, nil
	}
	if proxyFlag != nil && proxyFlag.Changed && noProxyFlag != nil && noProxyFlag.Changed {
		return "", fmt.Errorf("use either --proxy or --no-proxy, not both")
	}
	if noProxyFlag != nil && noProxyFlag.Changed {
		return "", nil
	}
	if proxyFlag != nil && proxyFlag.Changed {
		proxy, err := cmd.Flags().GetString("proxy")
		if err != nil {
			return "", fmt.Errorf("read --proxy: %w", err)
		}
		return proxy, nil
	}
	return runtimeConfig.HTTPSProxy, nil
}
