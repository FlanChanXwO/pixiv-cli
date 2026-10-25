// 配置快照到公开 SDK 选项的唯一翻译层，以及 CLI 运行时配置的读取入口。

package cli

import (
	"log/slog"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/httpclient"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func defaultCLIRuntimeConfig() (settings.RuntimeConfig, error) {
	snapshot, err := settings.DefaultStore().Current()
	if err != nil {
		return settings.RuntimeConfig{}, err
	}
	return snapshot.Runtime()
}

func pixivOptionsFromRequest(request pixivcmd.Request, loadRuntime func() (settings.RuntimeConfig, error), logger *slog.Logger) (pixiv.Options, error) {
	runtime, err := loadRuntime()
	if err != nil {
		return pixiv.Options{}, err
	}
	options := pixiv.Options{Logger: logger}
	options.Pacing.MinInterval = runtime.RequestInterval
	proxyValue := request.HTTPSProxyOverride
	if proxyValue == nil {
		if runtime.PixivNetwork.ProxyURL.Present {
			value := runtime.PixivNetwork.ProxyURL.Value
			proxyValue = &value
		} else {
			value := runtime.HTTPSProxy
			proxyValue = &value
		}
	}
	if proxyValue != nil {
		httpClient, err := httpclient.HTTPClient(*proxyValue)
		if err != nil {
			return pixiv.Options{}, err
		}
		options.HTTPClient = httpClient
	}
	return options, nil
}

// fanboxsdkOptions 是本包对公开 SDK options 类型的本地别名。保留别名的原因：
// 让 composition root 成为**唯一**把一份 config snapshot 翻译为 SDK options 的
// 地方，命令 owner 不需要（也不应该）直接依赖 SDK 的 options 形状。
type fanboxsdkOptions = fanbox.Options

func fanboxOptionsFromRuntime(loadRuntime func() (settings.RuntimeConfig, error), logger *slog.Logger) (fanboxsdkOptions, error) {
	cfg, err := loadRuntime()
	if err != nil {
		return fanboxsdkOptions{}, err
	}
	options := fanboxsdkOptions{Logger: logger}
	if cfg.FanboxNetwork.ProxyURL.Present {
		options.ProxyURL = cfg.FanboxNetwork.ProxyURL.Value
	} else {
		options.ProxyURL = cfg.HTTPSProxy
	}
	if cfg.FanboxNetwork.UserAgent.Present {
		options.UserAgent = cfg.FanboxNetwork.UserAgent.Value
	}
	if cfg.FanboxFlareSolverr != nil {
		options.FlareSolverr = &fanbox.FlareSolverrOptions{
			URL:      cfg.FanboxFlareSolverr.URL,
			ProxyURL: cfg.FanboxFlareSolverr.ProxyURL,
		}
	}
	return options, nil
}
