package update

import (
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	updateapp "github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/spf13/cobra"
)

// RunAutomaticCheck 在业务命令成功后展示稳定版更新提示。所有错误只写
// stderr，不改变已成功命令的退出码，也不进入 stdout/MCP JSON-RPC。
func RunAutomaticCheck(cmd *cobra.Command, host AutomaticCheckHost) {
	if !shouldCheck(cmd) {
		return
	}
	runtimeConfig, err := host.LoadAutomaticUpdateRuntimeConfig()
	if err != nil {
		warning(host, "load automatic update configuration", err)
		return
	}
	if !runtimeConfig.UpdateCheckEnabled {
		return
	}
	proxy, err := automaticProxy(cmd, runtimeConfig)
	if err != nil {
		warning(host, "read automatic update proxy override", err)
		return
	}
	checker, err := host.NewAutomaticUpdateChecker(proxy)
	if err != nil {
		warning(host, "create automatic update checker", err)
		return
	}
	notice, err := checker.Check(cmd.Context(), updateapp.AutomaticUpdateRequest{BuildInfo: buildinfo.Current()})
	if err != nil {
		warning(host, "check for updates", err)
		return
	}
	if notice == nil {
		return
	}
	_, _ = fmt.Fprintf(host.ErrorOutput(), "update available: %s -> %s\nrun: %s\n", notice.CurrentVersion, notice.LatestVersion, notice.UpdateCommand)
}

func warning(host AutomaticCheckHost, action string, err error) {
	_, _ = fmt.Fprintf(host.ErrorOutput(), "warning: %s: %v\n", action, err)
}

func shouldCheck(cmd *cobra.Command) bool {
	if buildinfo.Current().IsDevelopment() || cmd == nil || cmd.Root() == cmd {
		return false
	}
	if cmd.CommandPath() == "pixiv auth export" {
		return false
	}
	if cmd.CommandPath() == "pixiv auth import" {
		if pipeline.SkipAutomaticUpdate(cmd) {
			return false
		}
	}
	if helpFlag := cmd.Flags().Lookup("help"); helpFlag != nil && helpFlag.Changed {
		return false
	}
	if cmd.HasSubCommands() {
		return false
	}
	for current := cmd; current != nil; current = current.Parent() {
		switch current.Name() {
		case "help", "mcp", "version", "update", "_callback":
			return false
		}
	}
	return true
}

func automaticProxy(cmd *cobra.Command, runtimeConfig configapp.RuntimeConfig) (string, error) {
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
