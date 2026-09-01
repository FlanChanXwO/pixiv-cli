// Package assembly owns production construction of the reverse-search Facade.
// It is deliberately outside CLI/MCP owners so those adapters only depend on
// the top-level reversesearch contract and never import provider protocol code.
package assembly

import (
	"net/http"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch/ascii2d"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch/saucenao"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/network"
)

// Options are construction-time values for one CLI invocation. Proxy, the
// ascii2d user-agent, and the SauceNAO key never enter the per-request
// reversesearch.Request.
type Options struct {
	Proxy       string
	UserAgent   string
	SauceNAOKey string
}

// New constructs a Facade with a standard HTTP client for the source loader and
// SauceNAO. ascii2d receives only its proxy and user-agent so it can construct
// its dedicated browser transport without changing the other providers.
func New(options Options) (reversesearch.Searcher, error) {
	client, err := network.HTTPClient(options.Proxy)
	if err != nil {
		return nil, err
	}
	return newWithClient(options, client)
}

func newWithClient(options Options, client *http.Client) (reversesearch.Searcher, error) {
	return newWithClientWithASCII2DFactory(options, client, func(options ascii2d.Options) (reversesearch.ASCII2DClient, error) {
		return ascii2d.New(options)
	})
}

func newWithClientWithASCII2DFactory(
	options Options,
	client *http.Client,
	newASCII2DClient func(ascii2d.Options) (reversesearch.ASCII2DClient, error),
) (reversesearch.Searcher, error) {
	if client == nil {
		return nil, reversesearch.NewError(reversesearch.CodeProviderNotConfigured, "reverse search HTTP client is not configured", nil)
	}
	asciiClient, err := newASCII2DClient(ascii2d.Options{
		ProxyURL:  options.Proxy,
		UserAgent: options.UserAgent,
	})
	if err != nil {
		return nil, err
	}
	aggregator := reversesearch.NewAggregator(reversesearch.AggregatorDependencies{
		SauceNAO: saucenao.New(saucenao.Options{APIKey: options.SauceNAOKey, HTTPClient: client}),
		ASCII2D:  asciiClient,
	})
	return reversesearch.NewFacade(reversesearch.Dependencies{
		Sources:  reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{HTTPClient: client}),
		Payloads: aggregator,
	}), nil
}
