// Package assembly 负责在生产组合根装配反向搜图 [reversesearch.Service]。
//
// 本包位于 CLI/MCP owner 之外，让这些适配器只依赖顶层 reversesearch 契约，不导入
// provider 协议代码。
//
// 包的组成：[New] 按 [Options] 创建 Service，[FlareSolverrOptions] 配置可选的
// challenge 恢复服务。
package assembly

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/httpclient"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/ascii2d"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/saucenao"
)

// FlareSolverrOptions 配置 ascii2d 可选的 challenge 恢复服务；它的浏览器代理与反搜
// 标准 HTTP 代理分开设置。
type FlareSolverrOptions struct {
	URL      string
	ProxyURL string
}

// Options 提供一次 CLI 调用的构造值。Proxy 用于 source 载入与 SauceNAO 的标准 HTTP
// 代理；ASCII2DProxy 单独覆盖 ascii2d 的浏览器传输，nil 时沿用 Proxy。
type Options struct {
	Proxy        string
	ASCII2DProxy *string
	UserAgent    string
	SauceNAOKey  string
	FlareSolverr *FlareSolverrOptions
}

// New 用标准 HTTP client 构造 source 载入器与 SauceNAO；ascii2d 只收到代理与
// User-Agent，自行建立浏览器传输，不改变其他 provider。
func New(options Options) (reversesearch.Searcher, error) {
	client, err := httpclient.HTTPClient(options.Proxy)
	if err != nil {
		return nil, err
	}
	asciiOptions := ascii2d.Options{ProxyURL: options.ascii2dProxy(), UserAgent: options.UserAgent}
	if options.FlareSolverr != nil {
		asciiOptions.FlareSolverrURL = &options.FlareSolverr.URL
		asciiOptions.FlareSolverrProxyURL = options.FlareSolverr.ProxyURL
	}
	asciiClient, err := ascii2d.New(asciiOptions)
	if err != nil {
		return nil, err
	}
	aggregator := reversesearch.NewAggregator(
		saucenao.New(saucenao.Options{APIKey: options.SauceNAOKey, HTTPClient: client}),
		asciiClient,
	)
	return reversesearch.NewService(
		reversesearch.NewSourceLoader(client),
		aggregator,
	), nil
}

func (options Options) ascii2dProxy() string {
	if options.ASCII2DProxy != nil {
		return *options.ASCII2DProxy
	}
	return options.Proxy
}
