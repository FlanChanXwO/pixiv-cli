package e2e

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	reverseassembly "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch/assembly"
)

const (
	reverseSearchE2EEnabledEnv  = "PIXIV_REVERSE_SEARCH_E2E"
	reverseSearchE2ESourceEnv   = "PIXIV_REVERSE_SEARCH_SOURCE"
	reverseSearchE2EProviderEnv = "PIXIV_REVERSE_SEARCH_PROVIDER"
	reverseSearchE2EProxyEnv    = "PIXIV_REVERSE_SEARCH_PROXY"
	reverseSearchE2EKeyEnv      = "SAUCENAO_API_KEY"
)

type reverseSearchE2EConfig struct {
	enabled     bool
	source      string
	provider    reversesearch.Provider
	proxy       string
	sauceNAOKey string
}

func TestReverseSearchE2EConfigIsDisabledByDefault(t *testing.T) {
	config, err := reverseSearchE2EConfigFrom(func(string) string { return "" })
	if err != nil {
		t.Fatalf("disabled reverse-search e2e returned an error: %v", err)
	}
	if config.enabled {
		t.Fatal("reverse-search e2e enabled without explicit opt-in")
	}
}

func TestReverseSearchE2EConfigRequiresSourceWhenEnabled(t *testing.T) {
	const key = "saucenao-key-secret"
	config, err := reverseSearchE2EConfigFrom(func(name string) string {
		if name == reverseSearchE2EEnabledEnv {
			return "1"
		}
		if name == reverseSearchE2EProviderEnv {
			return "ascii2d-color"
		}
		if name == reverseSearchE2EKeyEnv {
			return key
		}
		return ""
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "source") {
		t.Fatalf("missing source error = %v", err)
	}
	if strings.Contains(err.Error(), key) {
		t.Fatalf("missing source error echoed key: %v", err)
	}
	if config.enabled {
		t.Fatal("invalid configuration was marked enabled")
	}
}

func TestReverseSearchE2EConfigRequiresSauceNAOKey(t *testing.T) {
	const source = "source-secret"
	config, err := reverseSearchE2EConfigFrom(func(name string) string {
		if name == reverseSearchE2EEnabledEnv {
			return "1"
		}
		if name == reverseSearchE2ESourceEnv {
			return source
		}
		if name == reverseSearchE2EProviderEnv {
			return "saucenao"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "SAUCENAO_API_KEY") {
		t.Fatalf("missing SauceNAO key error = %v", err)
	}
	if strings.Contains(err.Error(), source) {
		t.Fatalf("missing SauceNAO key error echoed source: %v", err)
	}
	if config.enabled {
		t.Fatal("invalid configuration was marked enabled")
	}
}

func TestReverseSearchE2EConfigAllowsAscii2DWithoutSauceNAOKey(t *testing.T) {
	config, err := reverseSearchE2EConfigFrom(func(name string) string {
		switch name {
		case reverseSearchE2EEnabledEnv:
			return "1"
		case reverseSearchE2ESourceEnv:
			return "https://example.test/image.png"
		case reverseSearchE2EProviderEnv:
			return "ascii2d-color"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("ascii2d-only configuration rejected: %v", err)
	}
	if !config.enabled || config.provider != reversesearch.ProviderASCII2DColor || config.sauceNAOKey != "" {
		t.Fatalf("ascii2d-only configuration = %+v", config)
	}
}

func TestRealReverseSearch(t *testing.T) {
	config, err := reverseSearchE2EConfigFrom(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if !config.enabled {
		t.Skip("set PIXIV_REVERSE_SEARCH_E2E=1 to run the real reverse-search e2e")
	}

	searcher, err := reverseassembly.New(reverseassembly.Options{Proxy: config.proxy, SauceNAOKey: config.sauceNAOKey})
	if err != nil {
		t.Fatalf("construct real reverse-search e2e client: code=%s", reversesearch.CodeOf(err))
	}
	response, err := searcher.Search(context.Background(), reversesearch.Request{
		Source: config.source, Provider: config.provider, PixivOnly: true,
	})
	if err != nil {
		t.Fatalf("real reverse-search e2e failed: code=%s", reversesearch.CodeOf(err))
	}
	if response.Input.Kind == "" || response.Input.SHA256 == "" || len(response.Providers) == 0 {
		t.Fatal("real reverse-search e2e returned an incomplete safe envelope")
	}
}

func reverseSearchE2EConfigFrom(getenv func(string) string) (reverseSearchE2EConfig, error) {
	if getenv(reverseSearchE2EEnabledEnv) != "1" {
		return reverseSearchE2EConfig{}, nil
	}
	config := reverseSearchE2EConfig{
		source:   strings.TrimSpace(getenv(reverseSearchE2ESourceEnv)),
		provider: reversesearch.Provider(strings.TrimSpace(getenv(reverseSearchE2EProviderEnv))),
		proxy:    strings.TrimSpace(getenv(reverseSearchE2EProxyEnv)),
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
