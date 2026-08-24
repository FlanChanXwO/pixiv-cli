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

// Options are construction-time values for one CLI invocation. Proxy and the
// SauceNAO key never enter the per-request reversesearch.Request.
type Options struct {
	Proxy       string
	SauceNAOKey string
}

// New constructs a Facade with one proxy-specific HTTP client shared by the
// source loader and provider clients. Provider requests still reopen the same
// private snapshot through the Facade contract.
func New(options Options) (reversesearch.Searcher, error) {
	client, err := network.HTTPClient(options.Proxy)
	if err != nil {
		return nil, err
	}
	return newWithClient(options, client)
}

func newWithClient(options Options, client *http.Client) (reversesearch.Searcher, error) {
	if client == nil {
		return nil, reversesearch.NewError(reversesearch.CodeProviderNotConfigured, "reverse search HTTP client is not configured", nil)
	}
	asciiClient, err := ascii2d.New(ascii2d.Options{HTTPClient: client})
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
