// 反向搜图的 CLI 与 MCP 装配：固定传输、凭据和 provider 默认值。

package cli

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/mcp"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/search"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/assembly"
)

// newReverseSearcher 固定 standard/ascii2d 两个代理网络面、凭据和 solver 配置。
func newReverseSearcher(runtime settings.RuntimeConfig, proxyOverride *string) (reversesearch.Searcher, error) {
	standardProxy, ascii2dProxy := runtime.HTTPSProxy, runtime.HTTPSProxy
	if runtime.ReverseSearchNetwork.ProxyURL.Present {
		ascii2dProxy = runtime.ReverseSearchNetwork.ProxyURL.Value
	}
	if proxyOverride != nil {
		standardProxy, ascii2dProxy = *proxyOverride, *proxyOverride
	}
	var flareSolverr *assembly.FlareSolverrOptions
	if runtime.ReverseSearchFlareSolverr != nil {
		flareSolverr = &assembly.FlareSolverrOptions{
			URL:      runtime.ReverseSearchFlareSolverr.URL,
			ProxyURL: runtime.ReverseSearchFlareSolverr.ProxyURL,
		}
	}
	return assembly.New(assembly.Options{
		Proxy:        standardProxy,
		ASCII2DProxy: &ascii2dProxy,
		UserAgent:    runtime.ReverseSearchNetwork.UserAgent.Value,
		SauceNAOKey:  runtime.SauceNAOAPIKey,
		FlareSolverr: flareSolverr,
	})
}

// newMCPReverseSearchPorts 在 MCP 启动时创建一次反向搜图 Facade，并
// 固定 standard/ascii2d 两个代理网络面、凭据和配置默认值。后续 tool input
// 只允许覆盖 provider，不能改变传输或 credential 依赖，避免同一 MCP session
// 的运行时语义漂移。
func newMCPReverseSearchPorts(runtime settings.RuntimeConfig, request mcp.Request) (pixiv.ReverseSearchPorts, error) {
	searcher, err := newReverseSearcher(runtime, request.HTTPSProxyOverride)
	if err != nil {
		return pixiv.ReverseSearchPorts{}, err
	}
	provider := reversesearch.Provider(runtime.ReverseSearchProvider)
	if provider == "" {
		provider = reversesearch.ProviderSauceNAO
	}
	return pixiv.ReverseSearchPorts{
		Searcher:  searcher,
		Provider:  provider,
		PixivOnly: runtime.ReverseSearchPixivOnly,
	}, nil
}

// reverseSearchPort builds the reverse-search executor once per command tree.
func (a app) reverseSearchPort() search.ReverseSearchPort {
	return func(ctx context.Context, request search.ReverseSearchRequest) (reversesearch.Response, error) {
		runtime, err := a.runtimeConfig()
		if err != nil {
			return reversesearch.Response{}, err
		}
		provider := reversesearch.Provider(runtime.ReverseSearchProvider)
		if request.Provider != "" {
			provider = request.Provider
		}
		searcher, err := newReverseSearcher(runtime, request.HTTPSProxyOverride)
		if err != nil {
			return reversesearch.Response{}, err
		}
		registerReverseSearchCloser(a.closeState, searcher)
		return searcher.Search(ctx, reversesearch.Request{
			Source: request.Source, Provider: provider, PixivOnly: runtime.ReverseSearchPixivOnly,
		})
	}
}
