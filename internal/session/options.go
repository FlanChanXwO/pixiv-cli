package session

import (
	"fmt"
	"log/slog"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/httpclient"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PixivOptions 把运行时配置与请求的代理覆盖翻译成公开 SDK 选项，并返回只读会话
// 缓存键：同一次读取只在代理、节奏与语言都相同时复用 client。
func PixivOptions(runtime settings.RuntimeConfig, request ProxyRequest, logger *slog.Logger) (PixivSettings, error) {
	options := pixiv.Options{Logger: logger}
	options.Pacing.MinInterval = runtime.RequestInterval
	proxy := runtime.HTTPSProxy
	if runtime.PixivNetwork.ProxyURL.Present {
		proxy = runtime.PixivNetwork.ProxyURL.Value
	}
	if request.HTTPSProxyOverride != nil {
		proxy = *request.HTTPSProxyOverride
	}
	httpClient, err := httpclient.HTTPClient(proxy)
	if err != nil {
		return PixivSettings{}, err
	}
	options.HTTPClient = httpClient
	return PixivSettings{
		Options: options,
		Network: fmt.Sprintf("%q/%d/%q", proxy, options.Pacing.MinInterval, options.AcceptLanguage),
	}, nil
}

// FanboxOptions 把运行时配置翻译成公开 FANBOX SDK 选项。
func FanboxOptions(runtime settings.RuntimeConfig, logger *slog.Logger) fanbox.Options {
	options := fanbox.Options{Logger: logger}
	if runtime.FanboxNetwork.ProxyURL.Present {
		options.ProxyURL = runtime.FanboxNetwork.ProxyURL.Value
	} else {
		options.ProxyURL = runtime.HTTPSProxy
	}
	if runtime.FanboxNetwork.UserAgent.Present {
		options.UserAgent = runtime.FanboxNetwork.UserAgent.Value
	}
	if runtime.FanboxFlareSolverr != nil {
		options.FlareSolverr = &fanbox.FlareSolverrOptions{
			URL:      runtime.FanboxFlareSolverr.URL,
			ProxyURL: runtime.FanboxFlareSolverr.ProxyURL,
		}
	}
	return options
}
