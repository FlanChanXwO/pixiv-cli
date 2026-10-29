package cli

import (
	"log/slog"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/httpclient"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
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
	proxy := runtime.HTTPSProxy
	if runtime.PixivNetwork.ProxyURL.Present {
		proxy = runtime.PixivNetwork.ProxyURL.Value
	}
	if request.HTTPSProxyOverride != nil {
		proxy = *request.HTTPSProxyOverride
	}
	httpClient, err := httpclient.HTTPClient(proxy)
	if err != nil {
		return pixiv.Options{}, err
	}
	options.HTTPClient = httpClient
	return options, nil
}

func fanboxOptionsFromRuntime(loadRuntime func() (settings.RuntimeConfig, error), logger *slog.Logger) (fanbox.Options, error) {
	cfg, err := loadRuntime()
	if err != nil {
		return fanbox.Options{}, err
	}
	options := fanbox.Options{Logger: logger}
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
