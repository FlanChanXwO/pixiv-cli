// Package assembly owns production construction of the reverse-search Service.
// It is deliberately outside CLI/MCP owners so those adapters only depend on
// the top-level reversesearch contract and never import provider protocol code.
package assembly

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/httpclient"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/ascii2d"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/saucenao"
)

// FlareSolverrOptions describes the optional challenge-recovery service for
// ascii2d. Its browser proxy is kept separate from the standard reverse-search
// HTTP proxy.
type FlareSolverrOptions struct {
	URL      string
	ProxyURL string
}

// Options are construction-time values for one CLI invocation. Proxy is the
// standard HTTP proxy for source loading and SauceNAO. ASCII2DProxy is an
// optional override for ascii2d's dedicated browser transport; when omitted,
// it preserves the historical behavior of reusing Proxy.
type Options struct {
	Proxy        string
	ASCII2DProxy *string
	UserAgent    string
	SauceNAOKey  string
	FlareSolverr *FlareSolverrOptions
}

// New constructs a Service with a standard HTTP client for the source loader and
// SauceNAO. ascii2d receives only its proxy and user-agent so it can construct
// its dedicated browser transport without changing the other providers.
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
