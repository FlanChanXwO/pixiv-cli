package session

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
)

// PixivOptions 把运行时配置与请求的代理覆盖翻译成公开 SDK 选项，并返回只读会话
// 缓存键：同一次读取只在代理、节奏与语言都相同时复用 client。
func PixivOptions(runtime config.Config, request ProxyRequest, logger *slog.Logger) (PixivSettings, error) {
	options := pixiv.Options{Logger: logger}
	interval, err := time.ParseDuration(runtime.Network.RequestInterval)
	if err != nil {
		return PixivSettings{}, err
	}
	options.Pacing.MinInterval = interval
	proxy := runtime.Network.HTTPSProxy
	if runtime.Pixiv.Network.ProxyURL != nil {
		proxy = *runtime.Pixiv.Network.ProxyURL
	}
	if request.HTTPSProxyOverride != nil {
		proxy = *request.HTTPSProxyOverride
	}
	httpClient, err := config.HTTPClient(proxy)
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
func FanboxOptions(runtime config.Config, logger *slog.Logger) fanbox.Options {
	options := fanbox.Options{Logger: logger}
	if runtime.Fanbox.Network.ProxyURL != nil {
		options.ProxyURL = *runtime.Fanbox.Network.ProxyURL
	} else {
		options.ProxyURL = runtime.Network.HTTPSProxy
	}
	if runtime.Fanbox.Network.UserAgent != nil {
		options.UserAgent = *runtime.Fanbox.Network.UserAgent
	}
	if runtime.Fanbox.FlareSolverr.URL != nil {
		options.FlareSolverr = &fanbox.FlareSolverrOptions{
			URL: *runtime.Fanbox.FlareSolverr.URL,
		}
		if runtime.Fanbox.FlareSolverr.ProxyURL != nil {
			options.FlareSolverr.ProxyURL = *runtime.Fanbox.FlareSolverr.ProxyURL
		}
	}
	return options
}
