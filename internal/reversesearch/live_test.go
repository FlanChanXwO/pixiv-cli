package reversesearch_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	reverseassembly "github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/assembly"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	reverseSearchE2EEnabledEnv     = "PIXIV_REVERSE_SEARCH_E2E"
	reverseSearchE2ESourceEnv      = "PIXIV_REVERSE_SEARCH_SOURCE"
	reverseSearchE2EProviderEnv    = "PIXIV_REVERSE_SEARCH_PROVIDER"
	reverseSearchE2EProxyEnv       = "PIXIV_REVERSE_SEARCH_PROXY"
	reverseSearchE2ESolverURLEnv   = "PIXIV_REVERSE_SEARCH_SOLVER_URL"
	reverseSearchE2ESolverProxyEnv = "PIXIV_REVERSE_SEARCH_SOLVER_PROXY"
	reverseSearchE2EKeyEnv         = "SAUCENAO_API_KEY"
)

type reverseSearchE2EConfig struct {
	enabled     bool
	source      string
	provider    reversesearch.Provider
	proxy       string
	solverURL   string
	solverProxy string
	sauceNAOKey string
}

func TestRealReverseSearch(t *testing.T) {
	config, err := reverseSearchE2EConfigFrom(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if !config.enabled {
		t.Skip("set PIXIV_REVERSE_SEARCH_E2E=1 to run the real reverse-search e2e")
	}

	var solverOptions *reverseassembly.FlareSolverrOptions
	if config.solverURL != "" {
		solverOptions = &reverseassembly.FlareSolverrOptions{
			URL: config.solverURL, ProxyURL: config.solverProxy,
		}
	}
	searcher, err := reverseassembly.New(reverseassembly.Options{
		Proxy: config.proxy, SauceNAOKey: config.sauceNAOKey, FlareSolverr: solverOptions,
	})
	if err != nil {
		t.Fatalf("construct real reverse-search e2e client: code=%s", reversesearch.CodeOf(err))
	}
	registerRealReverseSearchE2ECleanup(t, searcher)
	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: config.source, Provider: config.provider, PixivOnly: true,
	})
	if err != nil {
		t.Fatalf("real reverse-search e2e failed: code=%s", reversesearch.CodeOf(err))
	}
	if response.Input.Kind == "" || response.Input.SHA256 == "" || len(response.Providers) == 0 {
		t.Fatal("real reverse-search e2e returned an incomplete safe envelope")
	}
	if config.provider == reversesearch.ProviderAll {
		assertCompleteAllResponse(t, response.Providers, response.Partial, response.ProviderErrors)
	}
}

func TestRealReverseSearchMCPReusesSolverSession(t *testing.T) {
	config, err := reverseSearchE2EConfigFrom(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if !config.enabled {
		t.Skip("set PIXIV_REVERSE_SEARCH_E2E=1 to run the real reverse-search MCP e2e")
	}
	if config.provider != reversesearch.ProviderAll {
		t.Fatal("PIXIV_REVERSE_SEARCH_PROVIDER=all is required for the real reverse-search MCP e2e")
	}
	if config.solverURL == "" {
		t.Fatal("PIXIV_REVERSE_SEARCH_SOLVER_URL is required for the real reverse-search MCP e2e")
	}

	searcher, err := reverseassembly.New(reverseassembly.Options{
		Proxy:       config.proxy,
		SauceNAOKey: config.sauceNAOKey,
		FlareSolverr: &reverseassembly.FlareSolverrOptions{
			URL: config.solverURL, ProxyURL: config.solverProxy,
		},
	})
	if err != nil {
		t.Fatalf("construct real reverse-search MCP e2e client: code=%s", reversesearch.CodeOf(err))
	}
	session := newRealReverseSearchMCPSession(t, searcher)

	all := callRealReverseSearchMCP(t, session, config.source, reversesearch.ProviderAll)
	assertCompleteAllResponse(t, all.Providers, all.Partial, all.ProviderErrors)
	if all.Input.Kind == "" || all.Input.SHA256 == "" {
		t.Fatal("real reverse-search MCP all response returned an incomplete input envelope")
	}

	color := callRealReverseSearchMCP(t, session, config.source, reversesearch.ProviderASCII2DColor)
	if color.Partial || len(color.ProviderErrors) != 0 || len(color.Providers) != 1 ||
		color.Providers[0].Name != reversesearch.ProviderASCII2DColor ||
		color.Providers[0].Status != reversesearch.ProviderStatusSuccess {
		t.Fatalf("real reverse-search MCP follow-up response = %+v", color)
	}
}

type realReverseSearchMCPOutput struct {
	Input          reversesearch.Input             `json:"input"`
	Providers      []reversesearch.ProviderSummary `json:"providers"`
	ProviderErrors []reversesearch.ProviderError   `json:"provider_errors"`
	Partial        bool                            `json:"partial"`
}

func assertCompleteAllResponse(t *testing.T, providers []reversesearch.ProviderSummary, partial bool, providerErrors []reversesearch.ProviderError) {
	t.Helper()
	if partial {
		t.Fatalf("all-provider response is partial: providers=%+v errors=%+v", providers, providerErrors)
	}
	if len(providerErrors) != 0 {
		t.Fatalf("complete all-provider response contains errors: %+v", providerErrors)
	}
	want := []reversesearch.Provider{
		reversesearch.ProviderSauceNAO,
		reversesearch.ProviderASCII2DColor,
		reversesearch.ProviderASCII2DBOVW,
	}
	if len(providers) != len(want) {
		t.Fatalf("all-provider summaries = %+v, want %d providers", providers, len(want))
	}
	for index, provider := range want {
		if providers[index].Name != provider || providers[index].Status != reversesearch.ProviderStatusSuccess {
			t.Fatalf("all-provider summary[%d] = %+v, want %s/success", index, providers[index], provider)
		}
	}
}

func newRealReverseSearchMCPSession(t *testing.T, searcher reversesearch.Searcher) *mcp.ClientSession {
	t.Helper()
	server := mcpserver.New(pixivmcpserver.SDKPorts{
		ReverseSearch: pixivmcpserver.ReverseSearchPorts{
			Searcher: searcher, Provider: reversesearch.ProviderAll, PixivOnly: true,
		},
	}, pixivmcpserver.Account{}, fanbox.SDKPorts{}, nil)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "reverse-search-e2e", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		if serverErr := <-serverDone; serverErr != nil && !errors.Is(serverErr, context.Canceled) {
			t.Errorf("real reverse-search MCP server runner: %v", serverErr)
		}
		if closeErr := closeRealReverseSearchE2EClient(searcher); closeErr != nil {
			t.Errorf("close real reverse-search MCP e2e client: code=%s", reversesearch.CodeOf(closeErr))
		}
		t.Fatalf("connect real reverse-search MCP e2e client: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close real reverse-search MCP client session: %v", err)
		}
		cancel()
		if serverErr := <-serverDone; serverErr != nil && !errors.Is(serverErr, context.Canceled) {
			t.Errorf("real reverse-search MCP server runner: %v", serverErr)
		}
		if err := closeRealReverseSearchE2EClient(searcher); err != nil {
			t.Errorf("close real reverse-search MCP e2e client: code=%s", reversesearch.CodeOf(err))
		}
	})
	return session
}

func closeRealReverseSearchE2EClient(searcher reversesearch.Searcher) error {
	if closer, ok := searcher.(reversesearch.Closer); ok {
		return closer.Close()
	}
	return nil
}

func registerRealReverseSearchE2ECleanup(t *testing.T, searcher reversesearch.Searcher) {
	t.Helper()
	t.Cleanup(func() {
		if err := closeRealReverseSearchE2EClient(searcher); err != nil {
			t.Errorf("close real reverse-search e2e client: code=%s", reversesearch.CodeOf(err))
		}
	})
}

func callRealReverseSearchMCP(t *testing.T, session *mcp.ClientSession, source string, provider reversesearch.Provider) realReverseSearchMCPOutput {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "pixiv_reverse_search", Arguments: map[string]any{"source": source, "provider": string(provider)},
	})
	if err != nil {
		t.Fatalf("call real reverse-search MCP tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("real reverse-search MCP tool returned an error: %+v", result)
	}
	var output realReverseSearchMCPOutput
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal real reverse-search MCP output: %v", err)
	}
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("decode real reverse-search MCP output: %v", err)
	}
	return output
}

func reverseSearchE2EConfigFrom(getenv func(string) string) (reverseSearchE2EConfig, error) {
	if getenv(reverseSearchE2EEnabledEnv) != "1" {
		return reverseSearchE2EConfig{}, nil
	}
	config := reverseSearchE2EConfig{
		source:      strings.TrimSpace(getenv(reverseSearchE2ESourceEnv)),
		provider:    reversesearch.Provider(strings.TrimSpace(getenv(reverseSearchE2EProviderEnv))),
		proxy:       strings.TrimSpace(getenv(reverseSearchE2EProxyEnv)),
		solverURL:   strings.TrimSpace(getenv(reverseSearchE2ESolverURLEnv)),
		solverProxy: strings.TrimSpace(getenv(reverseSearchE2ESolverProxyEnv)),
	}
	if config.source == "" {
		return reverseSearchE2EConfig{}, errors.New("PIXIV_REVERSE_SEARCH_SOURCE is required when real reverse-search e2e is enabled")
	}
	if config.provider == "" {
		config.provider = reversesearch.ProviderAll
	}
	switch config.provider {
	case reversesearch.ProviderSauceNAO, reversesearch.ProviderASCII2DColor, reversesearch.ProviderASCII2DBOVW, reversesearch.ProviderAll:
	default:
		return reverseSearchE2EConfig{}, errors.New("PIXIV_REVERSE_SEARCH_PROVIDER must be saucenao, ascii2d-color, ascii2d-bovw, or all")
	}
	config.sauceNAOKey = getenv(reverseSearchE2EKeyEnv)
	if (config.provider == reversesearch.ProviderSauceNAO || config.provider == reversesearch.ProviderAll) && strings.TrimSpace(config.sauceNAOKey) == "" {
		return reverseSearchE2EConfig{}, errors.New("SAUCENAO_API_KEY is required for the selected real reverse-search e2e provider")
	}
	config.enabled = true
	return config, nil
}
