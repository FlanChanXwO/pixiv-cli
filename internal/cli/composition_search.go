// 反向搜图的 CLI 与 MCP 装配：固定传输、凭据和 provider 默认值。

package cli

import (
	"context"

	mcpcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/mcp"
	pixivsearch "github.com/FlanChanXwO/pixiv-cli/internal/cli/search"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	mcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	reverseassembly "github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/assembly"
)

func reverseSearchFlareSolverr(runtime configapp.RuntimeConfig) *reverseassembly.FlareSolverrOptions {
	if runtime.ReverseSearchFlareSolverr == nil {
		return nil
	}
	return &reverseassembly.FlareSolverrOptions{
		URL:      runtime.ReverseSearchFlareSolverr.URL,
		ProxyURL: runtime.ReverseSearchFlareSolverr.ProxyURL,
	}
}

func reverseSearchProxies(runtime configapp.RuntimeConfig, override *string) (standardProxy, ascii2dProxy string) {
	standardProxy = runtime.HTTPSProxy
	if override != nil {
		return *override, *override
	}
	if runtime.ReverseSearchNetwork.ProxyURL.Present {
		return standardProxy, runtime.ReverseSearchNetwork.ProxyURL.Value
	}
	return standardProxy, standardProxy
}

// newMCPReverseSearchPorts 在 MCP 启动时创建一次反向搜图 Facade，并
// 固定 standard/ascii2d 两个代理网络面、凭据和配置默认值。后续 tool input
// 只允许覆盖 provider，不能改变传输或 credential 依赖，避免同一 MCP session
// 的运行时语义漂移。
func newMCPReverseSearchPorts(runtime configapp.RuntimeConfig, request mcpcommands.Request) (mcpserver.ReverseSearchPorts, error) {
	standardProxy, ascii2dProxy := reverseSearchProxies(runtime, request.HTTPSProxyOverride)
	searcher, err := newCLIMCPReverseSearch(reverseassembly.Options{
		Proxy:        standardProxy,
		ASCII2DProxy: &ascii2dProxy,
		UserAgent:    runtime.ReverseSearchNetwork.UserAgent.Value,
		SauceNAOKey:  runtime.SauceNAOAPIKey,
		FlareSolverr: reverseSearchFlareSolverr(runtime),
	})
	if err != nil {
		return mcpserver.ReverseSearchPorts{}, err
	}
	provider := reversesearch.Provider(runtime.ReverseSearchProvider)
	if provider == "" {
		provider = reversesearch.ProviderSauceNAO
	}
	return mcpserver.ReverseSearchPorts{
		Searcher:  searcher,
		Provider:  provider,
		PixivOnly: runtime.ReverseSearchPixivOnly,
	}, nil
}

// reverseSearchPort builds the reverse-search executor once per command tree.
func (a app) reverseSearchPort() pixivsearch.ReverseSearchPort {
	return func(ctx context.Context, request pixivsearch.ReverseSearchRequest) (reversesearch.Response, error) {
		runtime, err := a.runtimeConfig()
		if err != nil {
			return reversesearch.Response{}, err
		}
		provider := reversesearch.Provider(runtime.ReverseSearchProvider)
		if request.Provider != "" {
			provider = request.Provider
		}
		standardProxy, ascii2dProxy := reverseSearchProxies(runtime, request.HTTPSProxyOverride)
		searcher, err := newCLIReverseSearch(reverseassembly.Options{
			Proxy:        standardProxy,
			ASCII2DProxy: &ascii2dProxy,
			UserAgent:    runtime.ReverseSearchNetwork.UserAgent.Value,
			SauceNAOKey:  runtime.SauceNAOAPIKey,
			FlareSolverr: reverseSearchFlareSolverr(runtime),
		})
		if err != nil {
			return reversesearch.Response{}, err
		}
		registerReverseSearchCloser(a.closeState, searcher)
		return searcher.Search(ctx, reversesearch.Request{
			Source: request.Source, Provider: provider, PixivOnly: runtime.ReverseSearchPixivOnly,
		})
	}
}
