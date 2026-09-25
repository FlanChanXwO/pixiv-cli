package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pixivdeps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth"
	mcpcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/mcp"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	reverseassembly "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch/assembly"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/internal/vector"
	sdkpixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rootReverseSearcherFunc func(context.Context, reversesearch.Request) (reversesearch.Response, error)

func (f rootReverseSearcherFunc) Search(ctx context.Context, request reversesearch.Request) (reversesearch.Response, error) {
	return f(ctx, request)
}

func TestImageSearchJSONDoesNotOpenPixivSDK(t *testing.T) {
	useTempPaths(t)
	oldSDK := newCLIPixivSDKPorts
	oldReverse := newCLIReverseSearch
	sdkOpened := false
	newCLIPixivSDKPorts = func(_ app) (pixivSDKPorts, error) {
		sdkOpened = true
		return pixivSDKPorts{}, errors.New("Pixiv SDK must not be opened for image search")
	}
	newCLIReverseSearch = func(_ reverseassembly.Options) (reversesearch.Searcher, error) {
		return rootReverseSearcherFunc(func(_ context.Context, request reversesearch.Request) (reversesearch.Response, error) {
			if request.Provider != reversesearch.ProviderSauceNAO || !request.PixivOnly {
				t.Fatalf("unexpected reverse search request: %+v", request)
			}
			return reversesearch.Response{
				Input: reversesearch.Input{Kind: reversesearch.SourceKindURL, SHA256: "deadbeef"},
			}, nil
		}), nil
	}
	t.Cleanup(func() {
		newCLIPixivSDKPorts = oldSDK
		newCLIReverseSearch = oldReverse
	})

	var stdout, stderr bytes.Buffer
	root := (app{in: strings.NewReader(""), out: &stdout, errOut: &stderr, closeState: &closeState{}}).newRootCommand()
	root.SetArgs([]string{"search", "https://example.test/image.jpg", "--json"})

	require.NoError(t, root.Execute(), stderr.String())
	assert.False(t, sdkOpened, "image search initialized Pixiv SDK/account DB")
}

func TestImageSearchFailureDoesNotLeakPrivateCauseAcrossCLIOutput(t *testing.T) {
	useTempPaths(t)
	oldReverse := newCLIReverseSearch
	newCLIReverseSearch = func(_ reverseassembly.Options) (reversesearch.Searcher, error) {
		return rootReverseSearcherFunc(func(_ context.Context, _ reversesearch.Request) (reversesearch.Response, error) {
			return reversesearch.Response{
				Input:     reversesearch.Input{Kind: reversesearch.SourceKindURL, SHA256: "deadbeef"},
				Providers: []reversesearch.ProviderSummary{{Name: reversesearch.ProviderAll, Status: reversesearch.ProviderStatusError}},
				ProviderErrors: []reversesearch.ProviderError{{
					Provider: reversesearch.ProviderAll, Code: reversesearch.CodeAllProvidersFailed, Message: "reverse search provider failed",
				}},
			}, reversesearch.NewError(reversesearch.CodeAllProvidersFailed, "reverse search provider failed", errors.New("api-key-secret source-secret upstream-body-secret csrf-secret location-secret"))
		}), nil
	}
	t.Cleanup(func() { newCLIReverseSearch = oldReverse })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "https://source-secret.example.test/image?token=api-key-secret", "--json"}, strings.NewReader(""), &stdout, &stderr)

	assert.Equal(t, 1, code, stderr.String())
	assert.Contains(t, stderr.String(), "reverse search provider failed")
	for _, secret := range []string{"source-secret.example.test", "api-key-secret", "upstream-body-secret", "csrf-secret", "location-secret"} {
		assert.NotContains(t, stdout.String()+stderr.String(), secret)
	}
}

func TestImageSearchBuildsReverseSearchWithCLIProxyOverride(t *testing.T) {
	useTempPaths(t)
	oldReverse := newCLIReverseSearch
	var captured reverseassembly.Options
	newCLIReverseSearch = func(options reverseassembly.Options) (reversesearch.Searcher, error) {
		captured = options
		return rootReverseSearcherFunc(func(_ context.Context, _ reversesearch.Request) (reversesearch.Response, error) {
			return reversesearch.Response{}, nil
		}), nil
	}
	t.Cleanup(func() { newCLIReverseSearch = oldReverse })

	var stdout, stderr bytes.Buffer
	root := (app{in: strings.NewReader(""), out: &stdout, errOut: &stderr, closeState: &closeState{}}).newRootCommand()
	root.SetArgs([]string{"search", "https://example.test/image.jpg", "--proxy", "http://127.0.0.1:7890", "--ndjson"})

	require.NoError(t, root.Execute(), stderr.String())
	assert.Equal(t, "http://127.0.0.1:7890", captured.Proxy)
	require.NotNil(t, captured.ASCII2DProxy)
	assert.Equal(t, "http://127.0.0.1:7890", *captured.ASCII2DProxy)
}

func TestMCPReverseSearchUsesStartupConfigAndProxySnapshot(t *testing.T) {
	oldReverse := newCLIMCPReverseSearch
	var captured reverseassembly.Options
	newCLIMCPReverseSearch = func(options reverseassembly.Options) (reversesearch.Searcher, error) {
		captured = options
		return rootReverseSearcherFunc(func(_ context.Context, _ reversesearch.Request) (reversesearch.Response, error) {
			return reversesearch.Response{}, nil
		}), nil
	}
	t.Cleanup(func() { newCLIMCPReverseSearch = oldReverse })

	proxy := "http://mcp-flag-proxy"
	ports, err := newMCPReverseSearchPorts(configapp.RuntimeConfig{
		HTTPSProxy:             "http://configured-proxy",
		ReverseSearchProvider:  "all",
		ReverseSearchPixivOnly: false,
		SauceNAOAPIKey:         "startup-key",
		ReverseSearchFlareSolverr: &configapp.FlareSolverrConfig{
			URL:      "http://solver.invalid",
			ProxyURL: "socks5://solver-proxy.invalid:1080",
		},
	}, mcpcommands.Request{HTTPSProxyOverride: &proxy})
	require.NoError(t, err)
	assert.Equal(t, "http://mcp-flag-proxy", captured.Proxy)
	require.NotNil(t, captured.ASCII2DProxy)
	assert.Equal(t, "http://mcp-flag-proxy", *captured.ASCII2DProxy)
	assert.Equal(t, "startup-key", captured.SauceNAOKey)
	require.NotNil(t, captured.FlareSolverr)
	assert.Equal(t, "http://solver.invalid", captured.FlareSolverr.URL)
	assert.Equal(t, "socks5://solver-proxy.invalid:1080", captured.FlareSolverr.ProxyURL)
	assert.Equal(t, reversesearch.ProviderAll, ports.Provider)
	assert.False(t, ports.PixivOnly)
	assert.NotNil(t, ports.Searcher)
}

func TestMCPReverseSearchUsesConfiguredServiceProxyBeforeGlobal(t *testing.T) {
	oldReverse := newCLIMCPReverseSearch
	var captured reverseassembly.Options
	newCLIMCPReverseSearch = func(options reverseassembly.Options) (reversesearch.Searcher, error) {
		captured = options
		return rootReverseSearcherFunc(func(_ context.Context, _ reversesearch.Request) (reversesearch.Response, error) {
			return reversesearch.Response{}, nil
		}), nil
	}
	t.Cleanup(func() { newCLIMCPReverseSearch = oldReverse })

	tests := []struct {
		name         string
		proxy        configapp.OptionalString
		wantStandard string
		wantASCII2D  string
	}{
		{name: "configured service proxy wins for ascii2d only", proxy: configapp.OptionalString{Present: true, Value: "http://reverse-search-proxy"}, wantStandard: "http://global-proxy", wantASCII2D: "http://reverse-search-proxy"},
		{name: "explicit empty service proxy means direct for ascii2d only", proxy: configapp.OptionalString{Present: true}, wantStandard: "http://global-proxy", wantASCII2D: ""},
		{name: "absent service proxy falls back to global for both", proxy: configapp.OptionalString{}, wantStandard: "http://global-proxy", wantASCII2D: "http://global-proxy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newMCPReverseSearchPorts(configapp.RuntimeConfig{
				HTTPSProxy: "http://global-proxy",
				ReverseSearchNetwork: configapp.ServiceNetworkConfig{
					ProxyURL: test.proxy,
				},
				ReverseSearchProvider:  "all",
				ReverseSearchPixivOnly: false,
			}, mcpcommands.Request{})
			require.NoError(t, err)
			assert.Equal(t, test.wantStandard, captured.Proxy)
			require.NotNil(t, captured.ASCII2DProxy)
			assert.Equal(t, test.wantASCII2D, *captured.ASCII2DProxy)
		})
	}
}

func TestMCPReverseSearchPassesConfiguredUserAgentToAssembly(t *testing.T) {
	oldReverse := newCLIMCPReverseSearch
	var captured reverseassembly.Options
	newCLIMCPReverseSearch = func(options reverseassembly.Options) (reversesearch.Searcher, error) {
		captured = options
		return rootReverseSearcherFunc(func(_ context.Context, _ reversesearch.Request) (reversesearch.Response, error) {
			return reversesearch.Response{}, nil
		}), nil
	}
	t.Cleanup(func() { newCLIMCPReverseSearch = oldReverse })

	_, err := newMCPReverseSearchPorts(configapp.RuntimeConfig{
		ReverseSearchNetwork: configapp.ServiceNetworkConfig{
			UserAgent: configapp.OptionalString{Present: true, Value: "fixture-user-agent"},
		},
	}, mcpcommands.Request{})
	require.NoError(t, err)
	assert.Equal(t, "fixture-user-agent", captured.UserAgent)
}

func TestCloseStateClosesInReverseOrderOnceAndJoinsErrors(t *testing.T) {
	firstErr := errors.New("first close")
	secondErr := errors.New("second close")
	order := make([]int, 0, 2)
	state := &closeState{}
	state.add(func() error { order = append(order, 1); return firstErr })
	state.add(func() error { order = append(order, 2); return secondErr })

	err := state.close()
	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, secondErr)
	assert.Equal(t, []int{2, 1}, order)

	require.Equal(t, err, state.close())
	assert.Equal(t, []int{2, 1}, order)
}

func TestBrokenPipeSignalsAreScopedByOutputProtocol(t *testing.T) {
	useTempPaths(t)
	oldSDK := newCLIPixivSDKPorts
	oldMCP := runMCPServer
	newCLIPixivSDKPorts = func(_ app) (pixivSDKPorts, error) {
		return pixivSDKPorts{}, errors.New("stop after pre-run")
	}
	runMCPServer = func(_ app, _ context.Context, _ mcpcommands.Request) error { return nil }
	t.Cleanup(func() {
		newCLIPixivSDKPorts = oldSDK
		runMCPServer = oldMCP
	})

	tests := []struct {
		name         string
		args         []string
		wantExit     int
		wantPipeline int
		wantMCP      int
	}{
		{name: "ndjson", args: []string{"pixiv", "search", "miku", "--ndjson"}, wantExit: 1, wantPipeline: 1},
		{name: "mcp", args: []string{"pixiv", "mcp"}, wantExit: 0, wantMCP: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pipelineEnabled, pipelineStopped := 0, 0
			mcpEnabled, mcpStopped := 0, 0
			enablePipeline := func() func() {
				pipelineEnabled++
				return func() { pipelineStopped++ }
			}
			enableMCP := func() func() {
				mcpEnabled++
				return func() { mcpStopped++ }
			}

			var stdout, stderr bytes.Buffer
			code := RunContextWithBrokenPipeSignals(context.Background(), test.args, strings.NewReader(""), &stdout, &stderr, enablePipeline, enableMCP)

			assert.Equal(t, test.wantExit, code, stderr.String())
			assert.Equal(t, test.wantPipeline, pipelineEnabled)
			assert.Equal(t, test.wantPipeline, pipelineStopped)
			assert.Equal(t, test.wantMCP, mcpEnabled)
			assert.Equal(t, test.wantMCP, mcpStopped)
		})
	}
}

func TestCommandAutoWritesNDJSONFallsBackWithInputAnnotation(t *testing.T) {
	pipeReader, pipeWriter, err := os.Pipe()
	require.NoError(t, err)
	defer pipeReader.Close()
	defer pipeWriter.Close()

	root := &cobra.Command{Use: "pixiv"}
	cmd := &cobra.Command{Use: "search", Annotations: map[string]string{pipeline.AnnotationKey: "text"}}
	root.AddCommand(cmd)

	assert.True(t, commandAutoWritesNDJSON(cmd, pipeWriter))
}

func TestRunNoArgsPrintsCoreHelpOnly(t *testing.T) {
	_, configPath := useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "Usage:")
	assert.Contains(t, stdout.String(), "auth")
	assert.Contains(t, stdout.String(), "config")
	assert.NotContains(t, stdout.String(), "\n  help ")
	assert.NotContains(t, stdout.String(), "completion")
	_, err := os.Stat(configPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestAuthServiceLoadErrorIsReturned(t *testing.T) {
	useTempPaths(t)
	old := newCLIAccountServices
	loadErr := errors.New("database schema version 3 is newer than binary schema version 1")
	newCLIAccountServices = func(app) (authcommands.AccountService, pixivaccount.LoginService, error) {
		return authcommands.AccountService{}, pixivaccount.LoginService{}, loadErr
	}
	t.Cleanup(func() { newCLIAccountServices = old })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "list"}, strings.NewReader(""), &stdout, &stderr)

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), loadErr.Error())
	assert.NotContains(t, stderr.String(), "pixiv account service is not configured")
}

func TestRunMCPDispatchesCoreTransportOptions(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		wantProxy        string
		wantProxySet     bool
		wantExit         int
		wantHandlerError string
	}{
		{
			name:         "proxy",
			args:         []string{"pixiv", "mcp", "--proxy", "http://flag-proxy"},
			wantProxy:    "http://flag-proxy",
			wantProxySet: true,
		},
		{
			name:         "no proxy",
			args:         []string{"pixiv", "mcp", "--no-proxy"},
			wantProxySet: true,
		},
		{
			name:         "empty proxy",
			args:         []string{"pixiv", "mcp", "--proxy", ""},
			wantProxySet: true,
		},
		{
			name:             "handler error",
			args:             []string{"pixiv", "mcp"},
			wantExit:         1,
			wantHandlerError: "mcp failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useTempPaths(t)
			old := runMCPServer
			t.Cleanup(func() { runMCPServer = old })

			var seenProxy *string
			runMCPServer = func(_ app, _ context.Context, request mcpcommands.Request) error {
				seenProxy = request.HTTPSProxyOverride
				if test.wantHandlerError != "" {
					return errors.New(test.wantHandlerError)
				}
				return nil
			}

			var stdout, stderr bytes.Buffer
			code := Run(test.args, strings.NewReader(""), &stdout, &stderr)
			assert.Equal(t, test.wantExit, code, stderr.String())
			if test.wantHandlerError != "" {
				assert.Contains(t, stderr.String(), test.wantHandlerError)
			}
			if test.wantProxySet {
				require.NotNil(t, seenProxy)
				assert.Equal(t, test.wantProxy, *seenProxy)
			} else {
				assert.Nil(t, seenProxy)
			}
		})
	}
}

func TestRootHelpOmitsRemovedPersistentFlags(t *testing.T) {
	root := newTestRootCommand(t)
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"--help"})

	require.NoError(t, root.Execute())
	assert.Contains(t, output.String(), "--help")
	assert.Contains(t, output.String(), "--version")
	assert.NotContains(t, output.String(), "--debug")
	assert.NotContains(t, output.String(), "--sleep-request")
}

func TestDiagnosticsLevelControlsMCPStderrWithoutTouchingStdout(t *testing.T) {
	for _, test := range []struct {
		name       string
		level      string
		format     string
		wantOutput bool
	}{
		{name: "info is silent", level: "info", format: "text"},
		{name: "debug text", level: "debug", format: "text", wantOutput: true},
		{name: "debug json", level: "debug", format: "json", wantOutput: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			useTempPaths(t)
			t.Setenv("PIXIV_LOG_LEVEL", test.level)
			t.Setenv("PIXIV_LOG_FORMAT", test.format)
			oldMCP := runMCPServer
			oldSupported := automaticPersistentHandlerSupported
			t.Cleanup(func() {
				runMCPServer = oldMCP
				automaticPersistentHandlerSupported = oldSupported
			})
			runMCPServer = func(_ app, _ context.Context, _ mcpcommands.Request) error { return nil }
			automaticPersistentHandlerSupported = func() bool { return false }

			var stdout, stderr bytes.Buffer
			code := Run([]string{"pixiv", "mcp"}, strings.NewReader(""), &stdout, &stderr)
			require.Zero(t, code, stderr.String())
			require.Empty(t, stdout.String(), "MCP must keep stdout for JSON-RPC")
			if !test.wantOutput {
				require.Empty(t, stderr.String())
				return
			}
			lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
			require.Len(t, lines, 2)
			if test.format == "text" {
				require.Contains(t, stderr.String(), "[Pixiv CLI]")
				return
			}
			for _, line := range lines {
				var record map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &record))
				require.Equal(t, "DEBUG", record["level"])
			}
		})
	}
}

func TestConfigCommandsRemainSilentWhenDiagnosticsAreEnabled(t *testing.T) {
	useTempPaths(t)
	t.Setenv("PIXIV_LOG_LEVEL", "debug")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "path"}, strings.NewReader(""), &stdout, &stderr)
	require.Zero(t, code, stderr.String())
	require.NotEmpty(t, stdout.String())
	require.Empty(t, stderr.String())
}

func TestPixivOptionsUseConfiguredRequestIntervalForPacing(t *testing.T) {
	want := 2 * time.Second
	options, err := pixivOptionsFromRequest(pixivdeps.Request{}, func() (configapp.RuntimeConfig, error) {
		return configapp.RuntimeConfig{RequestInterval: want}, nil
	})
	require.NoError(t, err)
	require.Equal(t, want, options.Pacing.MinInterval)
}

func TestRunMCPRejectsMalformedProxyWithoutLeakingInput(t *testing.T) {
	useTempPaths(t)
	proxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp", "--proxy", proxy}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "invalid proxy configuration")
	for _, secret := range []string{"proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret"} {
		assert.NotContains(t, stderr.String(), secret)
	}
}

func TestNonNetworkCommandsRejectProxyFlags(t *testing.T) {
	for _, args := range [][]string{
		{"pixiv", "auth", "list", "--proxy", "http://flag-proxy"},
		{"pixiv", "config", "path", "--no-proxy"},
	} {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			useTempPaths(t)

			var stdout, stderr bytes.Buffer
			code := Run(args, strings.NewReader(""), &stdout, &stderr)

			require.NotZero(t, code)
			assert.Contains(t, stderr.String(), "unknown option")
		})
	}
}

func TestRunParseErrorsSkipStartup(t *testing.T) {
	useTempPaths(t)

	oldCleanup := cleanupPendingWindowsUpdate
	oldSupported := automaticPersistentHandlerSupported
	oldEnsure := ensureURLSchemeRelay
	oldMCP := runMCPServer
	oldSDK := newCLIPixivSDKPorts
	t.Cleanup(func() {
		cleanupPendingWindowsUpdate = oldCleanup
		automaticPersistentHandlerSupported = oldSupported
		ensureURLSchemeRelay = oldEnsure
		runMCPServer = oldMCP
		newCLIPixivSDKPorts = oldSDK
	})

	cleanupCalls, ensureCalls, mcpCalls, serviceCalls := 0, 0, 0, 0
	cleanupPendingWindowsUpdate = func() error { cleanupCalls++; return nil }
	automaticPersistentHandlerSupported = func() bool { return true }
	ensureURLSchemeRelay = func(context.Context) error { ensureCalls++; return nil }
	runMCPServer = func(_ app, _ context.Context, _ mcpcommands.Request) error { mcpCalls++; return nil }
	newCLIPixivSDKPorts = func(_ app) (pixivSDKPorts, error) {
		serviceCalls++
		return pixivSDKPorts{}, nil
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp", "--unknown"}, strings.NewReader(""), &stdout, &stderr)

	assert.Equal(t, 2, code)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "error: unknown option '--unknown'\n", stderr.String())
	assert.Equal(t, 0, cleanupCalls)
	assert.Equal(t, 0, ensureCalls)
	assert.Equal(t, 0, mcpCalls)
	assert.Equal(t, 0, serviceCalls)
}

func TestRunStartupCleanupControlsMCP(t *testing.T) {
	for _, test := range []struct {
		name       string
		cleanupErr error
		wantCode   int
		wantOutput bool
	}{
		{name: "continues on success", wantCode: 0, wantOutput: true},
		{name: "stops on failure", cleanupErr: errors.New("pending backup cannot be removed"), wantCode: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			useTempPaths(t)
			oldCleanup := cleanupPendingWindowsUpdate
			oldSupported := automaticPersistentHandlerSupported
			oldMCP := runMCPServer
			t.Cleanup(func() {
				cleanupPendingWindowsUpdate = oldCleanup
				automaticPersistentHandlerSupported = oldSupported
				runMCPServer = oldMCP
			})

			cleanupCalls, mcpCalls := 0, 0
			cleanupPendingWindowsUpdate = func() error {
				cleanupCalls++
				return test.cleanupErr
			}
			automaticPersistentHandlerSupported = func() bool { return false }
			runMCPServer = func(_ app, _ context.Context, _ mcpcommands.Request) error {
				mcpCalls++
				return nil
			}

			var stdout, stderr bytes.Buffer
			code := Run([]string{"pixiv", "mcp"}, strings.NewReader(""), &stdout, &stderr)

			assert.Equal(t, test.wantCode, code, stderr.String())
			assert.Equal(t, 1, cleanupCalls)
			if test.wantOutput {
				assert.Equal(t, 1, mcpCalls)
				assert.Empty(t, stdout.String())
				assert.Empty(t, stderr.String())
			} else {
				assert.Zero(t, mcpCalls)
				assert.Contains(t, stderr.String(), "clean pending update")
			}
		})
	}
}

func newTestRootCommand(t *testing.T) *cobra.Command {
	t.Helper()
	a := app{
		in:     strings.NewReader(""),
		out:    io.Discard,
		errOut: io.Discard,
	}
	root := a.newRootCommand()
	t.Cleanup(func() {
		pipeline.Clear(root)
		authcommands.ClearInputState(root)
	})
	return root
}

func useTempPaths(t *testing.T) (string, string) {
	t.Helper()

	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		oldValue, hadValue := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		t.Cleanup(func() {
			if hadValue {
				require.NoError(t, os.Setenv(name, oldValue))
				return
			}
			require.NoError(t, os.Unsetenv(name))
		})
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	base := filepath.Join(home, paths.AppDataDirName)
	databasePath := database.DatabasePath(base)
	configPath := filepath.Join(base, "config.toml")
	t.Cleanup(paths.SetConfigFilePathForTest(configPath))
	return databasePath, configPath
}

type closeTrackingReverseSearcher struct {
	rootReverseSearcherFunc
	closeCalls atomic.Int32
	closeErr   error
}

func (s *closeTrackingReverseSearcher) Close() error {
	s.closeCalls.Add(1)
	return s.closeErr
}

func TestCLIReverseSearchClosesSearcherOnceAndKeepsJSONOnStdout(t *testing.T) {
	useTempPaths(t)
	oldReverse := newCLIReverseSearch
	closeErr := errors.New("reverse search close failed")
	searcher := &closeTrackingReverseSearcher{
		rootReverseSearcherFunc: func(context.Context, reversesearch.Request) (reversesearch.Response, error) {
			return reversesearch.Response{Input: reversesearch.Input{Kind: reversesearch.SourceKindURL, SHA256: "deadbeef"}}, nil
		},
		closeErr: closeErr,
	}
	newCLIReverseSearch = func(reverseassembly.Options) (reversesearch.Searcher, error) {
		return searcher, nil
	}
	t.Cleanup(func() { newCLIReverseSearch = oldReverse })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "search", "https://example.test/image.jpg", "--json"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), closeErr.Error())
	require.JSONEq(t, `{"input":{"kind":"url","sha256":"deadbeef"},"providers":[],"records":[],"results":[],"provider_errors":[],"partial":false}`, stdout.String())
	require.Equal(t, int32(1), searcher.closeCalls.Load())
}

func TestMCPReverseSearchRegistersSearcherForStdioLifetime(t *testing.T) {
	useTempPaths(t)
	oldConfig := loadCLIRuntimeConfig
	oldReverse := newCLIMCPReverseSearch
	oldSDK := newCLIPixivSDKPorts
	oldStdio := runMCPStdio
	closeErr := errors.New("mcp reverse search close failed")
	searcher := &closeTrackingReverseSearcher{closeErr: closeErr}

	loadCLIRuntimeConfig = func() (configapp.RuntimeConfig, error) {
		return configapp.RuntimeConfig{}, nil
	}
	newCLIMCPReverseSearch = func(reverseassembly.Options) (reversesearch.Searcher, error) {
		return searcher, nil
	}
	newCLIPixivSDKPorts = func(app) (pixivSDKPorts, error) {
		return pixivSDKPorts{}, nil
	}
	runMCPStdio = func(context.Context, *mcp.Server) error { return nil }
	t.Cleanup(func() {
		loadCLIRuntimeConfig = oldConfig
		newCLIMCPReverseSearch = oldReverse
		newCLIPixivSDKPorts = oldSDK
		runMCPStdio = oldStdio
	})

	var stdout, stderr bytes.Buffer
	a := app{
		in:         strings.NewReader(""),
		out:        &stdout,
		errOut:     &stderr,
		closeState: &closeState{},
	}
	require.NoError(t, a.runPixivMCP(context.Background(), mcpcommands.Request{}))
	require.ErrorIs(t, a.closeResources(), closeErr)
	require.ErrorIs(t, a.closeResources(), closeErr)
	require.Equal(t, int32(1), searcher.closeCalls.Load())
	require.Empty(t, stdout.String())
}

func TestVectorRebuildEmptyIndexWithoutRuntimeOrAuth(t *testing.T) {
	t.Setenv("PIXIV_VECTOR_PYTHON", filepath.Join(t.TempDir(), "missing-python"))
	databasePath, configPath := useTempPaths(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "vector", "rebuild"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stdout.String() != "embedded: 0\n" || stderr.Len() != 0 {
		t.Fatalf("empty rebuild: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, path := range []string{databasePath, configPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rebuild touched auth/config %s: %v", path, err)
		}
	}
}

func TestVectorLocalStagesWithoutAuthOrFakeEmbedding(t *testing.T) {
	t.Setenv("PIXIV_VECTOR_PYTHON", filepath.Join(t.TempDir(), "missing-python"))
	databasePath, configPath := useTempPaths(t)
	gallery := t.TempDir()
	if err := os.WriteFile(filepath.Join(gallery, "a.png"), []byte("image bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "vector", "sync", "local", gallery}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "embedding runtime unavailable") {
		t.Fatalf("sync must report staged-but-not-embedded: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "scanned: 1\nchanged: 1\n") {
		t.Fatalf("staging summary missing: %q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "vector", "status"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "assets: 1\nembeddings: 0\n") {
		t.Fatalf("status after staging: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, path := range []string{databasePath, configPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("local vector command touched auth/config state %q: %v", path, err)
		}
	}
}

func TestVectorSearchUsesLocalIndexWithoutPixivAuth(t *testing.T) {
	t.Setenv("PIXIV_VECTOR_PYTHON", filepath.Join(t.TempDir(), "missing-python"))
	databasePath, configPath := useTempPaths(t)
	dir, err := paths.AppDataDir()
	if err != nil {
		t.Fatal(err)
	}
	store, err := vector.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	key := vector.Key{Source: "local", ID: "/gallery/a.png"}
	if _, err := store.Upsert(context.Background(), vector.Asset{Key: key, Fingerprint: "a", TargetModel: vector.ModelID, TargetGeneration: vector.Generation}); err != nil {
		t.Fatal(err)
	}
	if err := store.PutEmbedding(context.Background(), key, "a", vector.ModelID, vector.Generation, []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "vector", "search", "white hair"}, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "embedding runtime unavailable") {
		t.Fatalf("search should reach local inference: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, path := range []string{databasePath, configPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("vector search touched auth/config state %q: %v", path, err)
		}
	}
}

func TestVectorSearchGroupsPixivArtworkByBestPage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test runtime uses a POSIX shell fixture")
	}
	useTempPaths(t)
	dir, err := paths.AppDataDir()
	require.NoError(t, err)
	store, err := vector.Open(dir)
	require.NoError(t, err)
	ctx := context.Background()
	for _, item := range []struct {
		key      vector.Key
		metadata string
		x, y     float32
	}{
		{vector.Key{Source: "pixiv", ID: "123", Page: 0}, `{"title":"weaker"}`, .6, .8},
		{vector.Key{Source: "pixiv", ID: "123", Page: 1}, `{"title":"best"}`, 1, 0},
		{vector.Key{Source: "pixiv", ID: "456", Page: 0}, `{"title":"other"}`, .8, .6},
		{vector.Key{Source: "local", ID: "/gallery/a.png"}, `{}`, .2, .98},
	} {
		_, err := store.Upsert(ctx, vector.Asset{Key: item.key, Fingerprint: "same", Metadata: []byte(item.metadata), TargetModel: vector.ModelID, TargetGeneration: vector.Generation})
		require.NoError(t, err)
		values := make([]float32, 768)
		values[0], values[1] = item.x, item.y
		require.NoError(t, store.PutEmbedding(ctx, item.key, "same", vector.ModelID, vector.Generation, values))
	}
	require.NoError(t, store.Close())
	query := make([]float32, 768)
	query[0] = 1
	response, err := json.Marshal(struct {
		Vector []float32 `json:"vector"`
	}{query})
	require.NoError(t, err)
	script := filepath.Join(t.TempDir(), "fake-python")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' '{\"ready\":true}'\nwhile IFS= read -r line; do printf '%s\\n' '"+string(response)+"'; done\n"), 0o700))
	t.Setenv("PIXIV_VECTOR_PYTHON", script)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "vector", "search", "white hair"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var results []struct {
		Source   string          `json:"source"`
		SourceID string          `json:"source_id"`
		Page     int             `json:"page_index"`
		Score    float64         `json:"score"`
		Metadata json.RawMessage `json:"metadata"`
		URL      string          `json:"url"`
	}
	decoder := json.NewDecoder(&stdout)
	for decoder.More() {
		var result struct {
			Source   string          `json:"source"`
			SourceID string          `json:"source_id"`
			Page     int             `json:"page_index"`
			Score    float64         `json:"score"`
			Metadata json.RawMessage `json:"metadata"`
			URL      string          `json:"url"`
		}
		require.NoError(t, decoder.Decode(&result))
		results = append(results, result)
	}
	require.Len(t, results, 3, "one result per Pixiv Artwork, every local asset retained")
	require.Equal(t, "pixiv", results[0].Source)
	require.Equal(t, "123", results[0].SourceID)
	require.Equal(t, 1, results[0].Page)
	require.Equal(t, `{"title":"best"}`, string(results[0].Metadata))
	require.Equal(t, "https://www.pixiv.net/artworks/123", results[0].URL)
	require.InDelta(t, 1, results[0].Score, 1e-6)
	require.Equal(t, "456", results[1].SourceID)
	require.Equal(t, 0, results[1].Page)
	require.Equal(t, "local", results[2].Source)
	require.Equal(t, "/gallery/a.png", results[2].SourceID)
	require.Greater(t, results[0].Score, results[1].Score)
	require.Greater(t, results[1].Score, results[2].Score)
}

// TestVectorObserverEndToEndThroughCompositionRoot 锁定 Task 18 的端到端契约：
// 经真实 root 组装运行一次普通 Pixiv 列表命令后，已取得的作品必须落进私有索引，
// 且观察不产生任何额外 App API 请求、不改变 stdout 与退出码。
func TestVectorObserverEndToEndThroughCompositionRoot(t *testing.T) {
	useTempPaths(t)
	dir, err := paths.AppDataDir()
	require.NoError(t, err)

	calls := 0
	transport := rootRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Path != "/v1/illust/ranking" {
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
		body := `{"illusts":[{"id":9701,"title":"observed artwork","type":"illust","page_count":1,"create_date":"2026-09-01T00:00:00Z","user":{"id":71,"name":"artist"},"image_urls":{"large":"https://i.pximg.net/img/9701_cover.jpg"}}],"next_url":null}`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := sdkpixiv.NewWith("test-access-token", sdkpixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	require.NoError(t, err)

	oldSDK := newCLIPixivSDKPorts
	newCLIPixivSDKPorts = func(app) (pixivSDKPorts, error) {
		return pixivSDKPorts{
			open: func(pixivdeps.Request) (*sdkpixiv.Client, error) { return client, nil },
			execute: func(ctx context.Context, _ pixivdeps.Request, attempt func(context.Context, *sdkpixiv.Client) (bool, error)) error {
				_, err := attempt(ctx, client)
				return err
			},
			jsonOut: func(*bool) (bool, error) { return false, nil },
		}, nil
	}
	t.Cleanup(func() { newCLIPixivSDKPorts = oldSDK })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "ranking", "--mode", "day", "--ndjson"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	require.Equal(t, 1, calls, "ranking makes exactly one upstream pass, and observation adds none")

	// 观察必须在命令结束后落进独立私有索引，且不含任何向量（观察不加载模型）。
	store, err := vector.Open(dir)
	require.NoError(t, err)
	defer store.Close()
	assets, embeddings, err := store.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, assets, "the fetched artwork must be observed into the index")
	require.Equal(t, 0, embeddings, "observation must never embed")
	asset, err := store.Get(context.Background(), vector.Key{Source: "pixiv", ID: "9701", Page: 0})
	require.NoError(t, err)
	require.Contains(t, string(asset.Metadata), `"url":"https://www.pixiv.net/artworks/9701"`)
}

// TestVectorObserverEndToEndSurvivesIndexFailure 证明观察失败不影响普通命令。
func TestVectorObserverEndToEndSurvivesIndexFailure(t *testing.T) {
	useTempPaths(t)
	// 只让索引不可用：把 vector.db 占位为目录，config/账号路径保持正常，
	// 从而确认失败隔离来自观察本身而非命令装配。
	dir, err := paths.AppDataDir()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vector.db"), 0o700))

	transport := rootRoundTripFunc(func(*http.Request) (*http.Response, error) {
		body := `{"illusts":[{"id":9801,"title":"x","type":"illust","page_count":1,"create_date":"2026-09-01T00:00:00Z","user":{"id":81,"name":"a"},"image_urls":{"large":"https://i.pximg.net/img/c.jpg"}}],"next_url":null}`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	client, err := sdkpixiv.NewWith("test-access-token", sdkpixiv.Options{HTTPClient: &http.Client{Transport: transport}})
	require.NoError(t, err)

	oldSDK := newCLIPixivSDKPorts
	newCLIPixivSDKPorts = func(app) (pixivSDKPorts, error) {
		return pixivSDKPorts{
			open: func(pixivdeps.Request) (*sdkpixiv.Client, error) { return client, nil },
			execute: func(ctx context.Context, _ pixivdeps.Request, attempt func(context.Context, *sdkpixiv.Client) (bool, error)) error {
				_, err := attempt(ctx, client)
				return err
			},
			jsonOut: func(*bool) (bool, error) { return false, nil },
		}, nil
	}
	t.Cleanup(func() { newCLIPixivSDKPorts = oldSDK })

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "ranking", "--mode", "day", "--ndjson"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, "an unusable index must not fail the command: %s", stderr.String())
	require.Contains(t, stdout.String(), "9801")
}

type rootRoundTripFunc func(*http.Request) (*http.Response, error)

func (f rootRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
