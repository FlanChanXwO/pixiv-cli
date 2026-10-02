package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
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
	mcpauth "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	mcpaccounts "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	reverseassembly "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch/assembly"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
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
	}{
		{name: "ndjson", args: []string{"pixiv", "search", "miku", "--ndjson"}, wantExit: 1, wantPipeline: 1},
		{name: "mcp", args: []string{"pixiv", "mcp"}, wantExit: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pipelineEnabled, pipelineStopped := 0, 0
			enablePipeline := func() func() {
				pipelineEnabled++
				return func() { pipelineStopped++ }
			}

			var stdout, stderr bytes.Buffer
			code := RunContextWithPipelineSignal(context.Background(), test.args, strings.NewReader(""), &stdout, &stderr, enablePipeline)

			assert.Equal(t, test.wantExit, code, stderr.String())
			assert.Equal(t, test.wantPipeline, pipelineEnabled)
			assert.Equal(t, test.wantPipeline, pipelineStopped)
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

func TestMCPReverseSearchRegistersSearcherForHTTPLifetime(t *testing.T) {
	useTempPaths(t)
	oldConfig := loadCLIRuntimeConfig
	oldReverse := newCLIMCPReverseSearch
	oldSDK := newCLIPixivSDKPorts
	oldHTTP := runMCPHTTP
	closeErr := errors.New("mcp reverse search close failed")
	searcher := &closeTrackingReverseSearcher{closeErr: closeErr}

	loadCLIRuntimeConfig = func() (configapp.RuntimeConfig, error) {
		return configapp.RuntimeConfig{MCPListenAddr: "127.0.0.1:8123", MCPBaseURL: "http://127.0.0.1:8123"}, nil
	}
	newCLIMCPReverseSearch = func(reverseassembly.Options) (reversesearch.Searcher, error) {
		return searcher, nil
	}
	newCLIPixivSDKPorts = func(app) (pixivSDKPorts, error) {
		return pixivSDKPorts{}, nil
	}
	runMCPHTTP = func(ctx context.Context, server *mcp.Server, listenAddr, baseURL string, store mcpauth.Store, out io.Writer, login *mcpaccounts.LoginManager) error {
		require.NotNil(t, login, "production HTTP must receive the shared login manager")
		clientTransport, serverTransport := mcp.NewInMemoryTransports()
		serverSession, err := server.Connect(ctx, serverTransport, nil)
		require.NoError(t, err)
		defer serverSession.Close()
		client := mcp.NewClient(&mcp.Implementation{Name: "root-test", Version: "1"}, nil)
		session, err := client.Connect(ctx, clientTransport, nil)
		require.NoError(t, err)
		defer session.Close()
		tools, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		names := make([]string, 0, len(tools.Tools))
		for _, tool := range tools.Tools {
			names = append(names, tool.Name)
		}
		require.Contains(t, names, "pixiv_search_illust")
		require.Contains(t, names, "fanbox_current_user")
		require.NotContains(t, names, "download")
		return nil
	}
	t.Cleanup(func() {
		loadCLIRuntimeConfig = oldConfig
		newCLIMCPReverseSearch = oldReverse
		newCLIPixivSDKPorts = oldSDK
		runMCPHTTP = oldHTTP
	})

	var stdout, stderr bytes.Buffer
	a := app{
		in:         strings.NewReader(""),
		out:        &stdout,
		errOut:     &stderr,
		closeState: &closeState{},
	}
	_, err := a.InitMCPAuth(t.Context(), false)
	require.NoError(t, err)
	require.NoError(t, a.runPixivMCP(context.Background(), mcpcommands.Request{}))
	require.ErrorIs(t, a.closeResources(), closeErr)
	require.ErrorIs(t, a.closeResources(), closeErr)
	require.Equal(t, int32(1), searcher.closeCalls.Load())
	require.Empty(t, stdout.String())
}

func TestMCPAuthInitPersistsPrivateVerifierOnce(t *testing.T) {
	_, configPath := useTempPaths(t)
	var stdout, stderr bytes.Buffer
	code := RunContext(context.Background(), []string{"pixiv", "mcp", "auth", "init"}, strings.NewReader(""), &stdout, &stderr)
	require.Equal(t, 0, code, "init failed: %s", stderr.String())
	secret := strings.TrimSpace(stdout.String())
	require.NotEmpty(t, secret)
	require.Empty(t, stderr.String())
	filename := filepath.Join(filepath.Dir(configPath), "mcp-state.json")
	body, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.False(t, bytes.Contains(body, []byte(secret)), "raw owner secret was persisted")
	var state struct {
		Version       int    `json:"version"`
		OwnerVerifier string `json:"owner_verifier"`
	}
	require.NoError(t, json.Unmarshal(body, &state))
	require.Equal(t, 1, state.Version)
	require.Len(t, state.OwnerVerifier, 64)
	stdout.Reset()
	stderr.Reset()
	code = RunContext(context.Background(), []string{"pixiv", "mcp", "auth", "init"}, strings.NewReader(""), &stdout, &stderr)
	require.NotEqual(t, 0, code)
	require.Empty(t, stdout.String(), "ordinary re-init must not reveal a secret")
	unchanged, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.True(t, bytes.Equal(body, unchanged), "ordinary re-init replaced state")
}

func TestMCPAuthResetPreservesClientsAndSelection(t *testing.T) {
	_, configPath := useTempPaths(t)
	var stdout, stderr bytes.Buffer
	run := func(args ...string) int {
		stdout.Reset()
		stderr.Reset()
		return RunContext(t.Context(), append([]string{"pixiv", "mcp", "auth", "init"}, args...), strings.NewReader(""), &stdout, &stderr)
	}
	require.Zero(t, run())
	oldSecret := strings.TrimSpace(stdout.String())
	filename := filepath.Join(filepath.Dir(configPath), "mcp-state.json")
	body, err := os.ReadFile(filename)
	require.NoError(t, err)
	var state map[string]any
	require.NoError(t, json.Unmarshal(body, &state))
	state["clients"] = map[string]any{"connector": map[string]any{"client_name": "Fixture", "redirect_uris": []any{"https://client.example/callback"}}}
	state["grants"] = map[string]any{"grant": map[string]any{"client_id": "connector", "resource": "https://server.example/mcp", "scope": "mcp", "refresh_hash": strings.Repeat("a", 64), "access_tokens": map[string]string{strings.Repeat("b", 64): "2026-09-28T01:00:00Z"}, "used_refresh_hashes": []string{}, "revoked": false}}
	state["selected_pixiv_user_id"] = 72
	body, err = json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filename, body, 0600))
	require.Zero(t, run("--reset"), "reset failed: %s", stderr.String())
	newSecret := strings.TrimSpace(stdout.String())
	require.NotEmpty(t, newSecret)
	require.False(t, oldSecret == newSecret, "reset reused the owner secret")
	var after map[string]any
	body, err = os.ReadFile(filename)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &after))
	require.Equal(t, state["clients"], after["clients"])
	require.Equal(t, float64(72), after["selected_pixiv_user_id"])
	require.Empty(t, after["grants"])
	require.False(t, state["owner_verifier"] == after["owner_verifier"], "owner generation did not change")
	require.False(t, bytes.Contains(body, []byte(oldSecret)) || bytes.Contains(body, []byte(newSecret)), "raw secret persisted")
	require.Empty(t, stderr.String())
}

func TestMCPAuthInitOutputFailureReportsSavedState(t *testing.T) {
	_, configPath := useTempPaths(t)
	reader, writer := io.Pipe()
	require.NoError(t, reader.Close())
	defer writer.Close()
	var stderr bytes.Buffer
	code := RunContext(t.Context(), []string{"pixiv", "mcp", "auth", "init"}, strings.NewReader(""), writer, &stderr)
	require.NotZero(t, code)
	_, err := os.Stat(filepath.Join(filepath.Dir(configPath), "mcp-state.json"))
	require.NoError(t, err, "output failure must not pretend to roll back a committed owner")
	require.Contains(t, stderr.String(), "saved")
	require.Contains(t, stderr.String(), "pixiv mcp auth init --reset")
}

func TestMCPHTTPPreflightFailsBeforeStarting(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing endpoints", nil, "--listen-addr"},
		{"missing owner", []string{"--listen-addr", "127.0.0.1:8123", "--base-url", "http://127.0.0.1:8123"}, "pixiv mcp auth init"},
		{"invalid canonical", []string{"--listen-addr", "127.0.0.1:8123", "--base-url", "relative"}, "canonical MCP base URL"},
		{"ephemeral port", []string{"--listen-addr", "127.0.0.1:0", "--base-url", "http://127.0.0.1:8123"}, "nonzero port"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useTempPaths(t)
			var stdout, stderr bytes.Buffer
			args := append([]string{"pixiv", "mcp"}, tc.args...)
			code := RunContext(t.Context(), args, strings.NewReader(""), &stdout, &stderr)
			require.NotZero(t, code)
			require.Contains(t, stderr.String(), tc.want)
			require.Empty(t, stdout.String())
		})
	}
}

func TestFanboxMCPCommandIsRemoved(t *testing.T) {
	useTempPaths(t)
	var stdout, stderr bytes.Buffer
	code := RunContext(t.Context(), []string{"pixiv", "fanbox", "mcp"}, strings.NewReader(""), &stdout, &stderr)
	require.NotZero(t, code)
	require.Contains(t, stderr.String(), "usage: pixiv fanbox <command>")
}

func TestMCPHTTPEndpointFlagPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name         string
		args         []string
		listen, base string
	}{
		{"config", nil, "127.0.0.1:8123", "http://127.0.0.1:8123"},
		{"listen override", []string{"--listen-addr", "127.0.0.1:8124"}, "127.0.0.1:8124", "http://127.0.0.1:8123"},
		{"base override", []string{"--base-url", "https://public.test/prefix"}, "127.0.0.1:8123", "https://public.test/prefix"},
		{"empty listen", []string{"--listen-addr="}, "", ""},
		{"empty base", []string{"--base-url="}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, configPath := useTempPaths(t)
			var stdout, stderr bytes.Buffer
			a := app{in: strings.NewReader(""), out: &stdout, errOut: &stderr}
			_, err := a.InitMCPAuth(t.Context(), false)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(configPath, []byte(`[mcp]
listen_addr = "127.0.0.1:8123"
base_url = "http://127.0.0.1:8123"
`), 0600))
			oldHTTP, oldSDK := runMCPHTTP, newCLIPixivSDKPorts
			t.Cleanup(func() { runMCPHTTP, newCLIPixivSDKPorts = oldHTTP, oldSDK })
			newCLIPixivSDKPorts = func(app) (pixivSDKPorts, error) { return pixivSDKPorts{}, nil }
			called := false
			runMCPHTTP = func(_ context.Context, _ *mcp.Server, listen, base string, _ mcpauth.Store, out io.Writer, login *mcpaccounts.LoginManager) error {
				called = true
				require.Equal(t, tc.listen, listen)
				require.Equal(t, tc.base, base)
				require.Same(t, &stderr, out)
				return nil
			}
			code := RunContext(t.Context(), append([]string{"pixiv", "mcp"}, tc.args...), strings.NewReader(""), &stdout, &stderr)
			if tc.listen == "" {
				require.NotZero(t, code)
				require.False(t, called)
				require.Contains(t, stderr.String(), "MCP requires")
			} else {
				require.Equal(t, 0, code, stderr.String())
				require.True(t, called)
			}
			require.Empty(t, stdout.String())
		})
	}
}

func TestMCPProductionResolvesSharedSelectionBeforeSDK(t *testing.T) {
	useTempPaths(t)
	oldConfig, oldSDK, oldHTTP, oldReverse := loadCLIRuntimeConfig, newCLIPixivSDKPorts, runMCPHTTP, newCLIMCPReverseSearch
	t.Cleanup(func() {
		loadCLIRuntimeConfig, newCLIPixivSDKPorts, runMCPHTTP, newCLIMCPReverseSearch = oldConfig, oldSDK, oldHTTP, oldReverse
	})
	loadCLIRuntimeConfig = func() (configapp.RuntimeConfig, error) {
		return configapp.RuntimeConfig{MCPListenAddr: "127.0.0.1:8123", MCPBaseURL: "http://127.0.0.1:8123"}, nil
	}
	newCLIMCPReverseSearch = func(reverseassembly.Options) (reversesearch.Searcher, error) {
		return rootReverseSearcherFunc(func(context.Context, reversesearch.Request) (reversesearch.Response, error) {
			return reversesearch.Response{}, nil
		}), nil
	}
	local := mcpaccounts.LocalSnapshot{DefaultUserID: 42, Accounts: []mcpaccounts.Account{{UserID: 42, HasCredentials: true}, {UserID: 73, HasCredentials: true}}}
	var ids []int64
	newCLIPixivSDKPorts = func(app) (pixivSDKPorts, error) {
		return pixivSDKPorts{
			localAccounts: func(context.Context) (mcpaccounts.LocalSnapshot, error) { return local, nil },
			execute: func(_ context.Context, req pixivdeps.Request, _ func(context.Context, *sdkpixiv.Client) (bool, error)) error {
				ids = append(ids, req.UserID)
				return errors.New("fixture SDK boundary")
			},
		}, nil
	}
	runMCPHTTP = func(ctx context.Context, server *mcp.Server, _, _ string, store mcpauth.Store, _ io.Writer, login *mcpaccounts.LoginManager) error {

		ct, st := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, st, nil)
		require.NoError(t, err)
		defer ss.Close()
		client := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
		session, err := client.Connect(ctx, ct, nil)
		require.NoError(t, err)
		defer session.Close()
		switched, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pixiv_account_use", Arguments: map[string]any{"user_id": 73}})
		require.NoError(t, err)
		require.False(t, switched.IsError)
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pixiv_illust_detail", Arguments: map[string]any{"illust_id": 1}})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Equal(t, []int64{73}, ids)
		local.Accounts = local.Accounts[:1]
		result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "pixiv_illust_detail", Arguments: map[string]any{"illust_id": 1}})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Equal(t, []int64{73}, ids, "removed selection reached SDK or fell back to default")
		return nil
	}
	a := app{in: strings.NewReader(""), out: io.Discard, errOut: io.Discard, closeState: &closeState{}}
	_, err := a.InitMCPAuth(t.Context(), false)
	require.NoError(t, err)
	require.NoError(t, a.runPixivMCP(t.Context(), mcpcommands.Request{}))
	require.NoError(t, a.closeResources())
}

func TestMCPAccountSnapshotContainsOnlyLocalSummaryAndExplicitDefault(t *testing.T) {
	dbPath, _ := useTempPaths(t)
	db, err := database.Open(filepath.Dir(dbPath))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.SavePixivCredential(t.Context(), pixivaccount.New(42, "fixture-a", []byte("fixture-secret-a"))))
	require.NoError(t, db.SavePixivCredential(t.Context(), pixivaccount.New(73, "fixture-b", []byte("fixture-secret-b"))))
	a := app{closeState: &closeState{}}
	ports, err := a.newPixivSDKPorts()
	require.NoError(t, err)
	defer a.closeResources()
	local, err := ports.localAccounts(t.Context())
	require.NoError(t, err)
	require.Zero(t, local.DefaultUserID, "implicit first account must not become explicit default")
	require.Len(t, local.Accounts, 2)
	for _, account := range local.Accounts {
		require.True(t, account.HasCredentials)
	}
	body, err := json.Marshal(local)
	require.NoError(t, err)
	require.NotContains(t, string(body), "fixture-secret")
	require.NoError(t, configapp.SetPixivDefaultUserID(73))
	local, err = ports.localAccounts(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(73), local.DefaultUserID)
}
