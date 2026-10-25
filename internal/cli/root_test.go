package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	pixivdeps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	mcpcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/mcp"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	unifiedmcp "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	mcpauth "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	mcpaccounts "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	reverseassembly "github.com/FlanChanXwO/pixiv-cli/internal/reversesearch/assembly"
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
	root := (newTestApp(strings.NewReader(""), &stdout, &stderr)).newRootCommand()
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
	root := (newTestApp(strings.NewReader(""), &stdout, &stderr)).newRootCommand()
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
		ReverseSearchNetwork: configapp.ServiceNetworkConfig{
			UserAgent: configapp.OptionalString{Present: true, Value: "fixture-user-agent"},
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
	assert.Equal(t, "fixture-user-agent", captured.UserAgent)
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
		{name: "http", args: []string{"pixiv", "mcp", "--port", "1270"}, wantExit: 0},
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
	cmd := &cobra.Command{Use: "search", Annotations: map[string]string{input.AnnotationKey: "text"}}
	root.AddCommand(cmd)

	assert.True(t, commandAutoWritesNDJSON(cmd, pipeWriter))
}

func TestRootLicensesPrintsEmbeddedNoticesWithoutCreatingState(t *testing.T) {
	databasePath, configPath := useTempPaths(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "--licenses"}, strings.NewReader(""), &stdout, &stderr)
	require.Zero(t, code, stderr.String())
	require.Empty(t, stderr.String())
	for _, text := range []string{"quantette", "Permission is hereby granted", "The Go Authors", "Sun Microsystems"} {
		assert.Contains(t, stdout.String(), text)
	}
	for _, path := range []string{databasePath, configPath} {
		_, err := os.Stat(path)
		require.True(t, os.IsNotExist(err), "license output created %s", path)
	}
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

func TestLogLevelControlsMCPStderrWithoutTouchingStdout(t *testing.T) {
	for _, test := range []struct {
		name       string
		level      string
		format     string
		wantOutput bool
	}{
		{name: "info logs terminal errors only", level: "info", format: "text", wantOutput: true},
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
			runMCPServer = func(_ app, _ context.Context, _ mcpcommands.Request) error { return errors.New("fixture failure") }
			automaticPersistentHandlerSupported = func() bool { return false }

			var stdout, stderr bytes.Buffer
			code := Run([]string{"pixiv", "mcp"}, strings.NewReader(""), &stdout, &stderr)
			require.Equal(t, 1, code, stderr.String())
			require.Empty(t, stdout.String(), "MCP must keep stdout for JSON-RPC")
			require.Contains(t, stderr.String(), "error: fixture failure")
			if test.level == "info" {
				require.Contains(t, stderr.String(), "ERROR")
				return
			}
			output := strings.TrimSuffix(stderr.String(), "error: fixture failure\n")
			lines := strings.Split(strings.TrimSpace(output), "\n")
			require.NotEmpty(t, lines)
			if test.format == "text" {
				require.Contains(t, stderr.String(), "ERROR")
				require.Contains(t, stderr.String(), "operation=pixiv mcp")
				require.Contains(t, stderr.String(), "caused by: fixture failure")
				return
			}
			var record map[string]any
			require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
			require.Equal(t, "ERROR", record["level"])
			require.Equal(t, "pixiv mcp", record["operation"])
		})
	}
}

func TestConfigCommandsRemainSilentWhenLoggingIsEnabled(t *testing.T) {
	useTempPaths(t)
	t.Setenv("PIXIV_LOG_LEVEL", "debug")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "path"}, strings.NewReader(""), &stdout, &stderr)
	require.Zero(t, code, stderr.String())
	require.NotEmpty(t, stdout.String())
	require.Empty(t, stderr.String())
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

// newTestApp 构造只带流、关闭表和丢弃 logger 的 app；测试不经过 startLogging。
func newTestApp(in io.Reader, out, errOut io.Writer) app {
	return app{in: in, out: out, errOut: errOut, closeState: &closeState{}, logs: &logState{ctx: context.Background(), logger: logging.Discard()}}
}

func newTestRootCommand(t *testing.T) *cobra.Command {
	t.Helper()
	a := newTestApp(strings.NewReader(""), io.Discard, io.Discard)
	root := a.newRootCommand()
	t.Cleanup(func() {
		input.Clear(root)
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
		return configapp.RuntimeConfig{}, nil
	}
	newCLIMCPReverseSearch = func(reverseassembly.Options) (reversesearch.Searcher, error) {
		return searcher, nil
	}
	newCLIPixivSDKPorts = func(app) (pixivSDKPorts, error) {
		return pixivSDKPorts{}, nil
	}
	runMCPHTTP = func(ctx context.Context, server *mcp.Server, listenAddr, baseURL string, store mcpauth.Store, out io.Writer, login *mcpaccounts.Login, media ...*mediaproxy.Service) error {
		require.NotNil(t, login, "production HTTP must receive the shared login relay")
		clientTransport, serverTransport := mcp.NewInMemoryTransports()
		serverSession, err := server.Connect(ctx, serverTransport, nil)
		require.NoError(t, err)
		defer serverSession.Close()
		client := mcp.NewClient(&mcp.Implementation{Name: "root-test", Version: "1"}, nil)
		session, err := client.Connect(ctx, clientTransport, nil)
		require.NoError(t, err)
		defer session.Close()
		return nil
	}
	t.Cleanup(func() {
		loadCLIRuntimeConfig = oldConfig
		newCLIMCPReverseSearch = oldReverse
		newCLIPixivSDKPorts = oldSDK
		runMCPHTTP = oldHTTP
	})

	var stdout, stderr bytes.Buffer
	a := newTestApp(strings.NewReader(""), &stdout, &stderr)
	_, err := a.InitMCPAuth(t.Context(), false)
	require.NoError(t, err)
	require.NoError(t, a.runPixivMCP(context.Background(), mcpcommands.Request{ListenAddr: new("127.0.0.1:8123"), BaseURL: new("http://127.0.0.1:8123")}))
	require.ErrorIs(t, a.closeResources(), closeErr)
	require.ErrorIs(t, a.closeResources(), closeErr)
	require.Equal(t, int32(1), searcher.closeCalls.Load())
	require.Empty(t, stdout.String())
}

func TestMCPShutdownPreservesAnimationCleanupError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires POSIX directory permission enforcement")
	}
	useTempPaths(t)
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	oldConfig, oldSDK, oldHTTP, oldReverse := loadCLIRuntimeConfig, newCLIPixivSDKPorts, runMCPHTTP, newCLIMCPReverseSearch
	t.Cleanup(func() {
		loadCLIRuntimeConfig, newCLIPixivSDKPorts, runMCPHTTP, newCLIMCPReverseSearch = oldConfig, oldSDK, oldHTTP, oldReverse
	})
	loadCLIRuntimeConfig = func() (configapp.RuntimeConfig, error) {
		return configapp.RuntimeConfig{MCPGalleryAnimationTempCapacityMiB: 1}, nil
	}
	newCLIMCPReverseSearch = func(reverseassembly.Options) (reversesearch.Searcher, error) {
		return &closeTrackingReverseSearcher{}, nil
	}
	newCLIPixivSDKPorts = func(app) (pixivSDKPorts, error) {
		return pixivSDKPorts{readSession: func(context.Context, pixivdeps.Request) (mediaproxy.Session, error) {
			return mediaproxy.Session{}, errors.New("unexpected SDK access")
		}}, nil
	}
	serveErr := errors.New("fixture HTTP stopped")
	runMCPHTTP = func(context.Context, *mcp.Server, string, string, mcpauth.Store, io.Writer, *mcpaccounts.Login, ...*mediaproxy.Service) error {
		entries, err := os.ReadDir(temporary)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		workspace := filepath.Join(temporary, entries[0].Name())
		require.NoError(t, os.Chmod(workspace, 0500))
		t.Cleanup(func() { require.NoError(t, os.Chmod(workspace, 0700)) })
		return serveErr
	}
	a := newTestApp(strings.NewReader(""), io.Discard, io.Discard)
	_, err := a.InitMCPAuth(t.Context(), false)
	require.NoError(t, err)
	err = a.runPixivMCP(t.Context(), mcpcommands.Request{ListenAddr: new("127.0.0.1:8123"), BaseURL: new("http://127.0.0.1:8123")})
	require.ErrorIs(t, err, serveErr)
	require.ErrorIs(t, err, os.ErrPermission, "server failure must retain the animation cleanup failure")
	require.NoError(t, a.closeResources())
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
	registeredClient := map[string]any{"client_name": "Fixture", "redirect_uris": []any{"https://client.example/callback"}}
	state["clients"] = map[string]any{"connector": registeredClient}
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
	registeredClient["owner_approved"] = true
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
		{"explicit HTTP requires owner", []string{"--port", "1270"}, "pixiv mcp auth init"},
		{"missing owner", []string{"--port", "8123", "--base-url", "http://127.0.0.1:8123"}, "pixiv mcp auth init"},
		{"invalid canonical", []string{"--port", "8123", "--base-url", "relative"}, "canonical MCP base URL"},
		{"ephemeral port", []string{"--port", "0", "--base-url", "http://127.0.0.1:8123"}, "--port must"},
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

func TestMCPHTTPEndpointFlagPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name         string
		args         []string
		listen, base string
	}{
		{"defaults", []string{"--port", "1270"}, "127.0.0.1:1270", "http://127.0.0.1:1270"},
		{"config ignored", []string{"--port", "9000"}, "127.0.0.1:9000", "http://127.0.0.1:9000"},
		{"IPv6", []string{"--port", "8124", "--host", "::1"}, "[::1]:8124", "http://[::1]:8124"},
		{"base override", []string{"--port", "8123", "--base-url", "https://public.test/prefix/"}, "127.0.0.1:8123", "https://public.test/prefix"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, configPath := useTempPaths(t)
			var stdout, stderr bytes.Buffer
			a := newTestApp(strings.NewReader(""), &stdout, &stderr)
			_, err := a.InitMCPAuth(t.Context(), false)
			require.NoError(t, err)
			if tc.name != "defaults" {
				require.NoError(t, os.WriteFile(configPath, []byte(`[mcp]
listen_addr = "127.0.0.1:8123"
base_url = "http://127.0.0.1:8123"
`), 0600))
			}
			oldHTTP, oldSDK := runMCPHTTP, newCLIPixivSDKPorts
			t.Cleanup(func() { runMCPHTTP, newCLIPixivSDKPorts = oldHTTP, oldSDK })
			watchCalls := 0
			newCLIPixivSDKPorts = func(app) (pixivSDKPorts, error) {
				return pixivSDKPorts{watchReadSessions: func(context.Context) error { watchCalls++; return nil }}, nil
			}
			called := false
			runMCPHTTP = func(_ context.Context, _ *mcp.Server, listen, base string, _ mcpauth.Store, out io.Writer, login *mcpaccounts.Login, media ...*mediaproxy.Service) error {
				called = true
				require.Equal(t, tc.listen, listen)
				require.Equal(t, tc.base, base)
				require.Same(t, &stderr, out)
				return nil
			}
			code := RunContext(t.Context(), append([]string{"pixiv", "mcp"}, tc.args...), strings.NewReader(""), &stdout, &stderr)
			require.Equal(t, 0, code, stderr.String())
			require.True(t, called)
			require.Equal(t, 1, watchCalls)
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
		return configapp.RuntimeConfig{}, nil
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
	runMCPHTTP = func(ctx context.Context, server *mcp.Server, _, _ string, store mcpauth.Store, _ io.Writer, login *mcpaccounts.Login, media ...*mediaproxy.Service) error {

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
	a := newTestApp(strings.NewReader(""), io.Discard, io.Discard)
	_, err := a.InitMCPAuth(t.Context(), false)
	require.NoError(t, err)
	require.NoError(t, a.runPixivMCP(t.Context(), mcpcommands.Request{ListenAddr: new("127.0.0.1:8123"), BaseURL: new("http://127.0.0.1:8123")}))
	require.NoError(t, a.closeResources())
}

func TestMCPAccountSnapshotContainsOnlyLocalSummaryAndExplicitDefault(t *testing.T) {
	dbPath, _ := useTempPaths(t)
	db, err := database.Open(filepath.Dir(dbPath))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.SavePixivCredential(t.Context(), pixivaccount.New(42, "fixture-a", []byte("fixture-secret-a"))))
	require.NoError(t, db.SavePixivCredential(t.Context(), pixivaccount.New(73, "fixture-b", []byte("fixture-secret-b"))))
	a := newTestApp(nil, io.Discard, io.Discard)
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
	require.NoError(t, configapp.DefaultStore().SetPixivDefaultUserID(73))
	local, err = ports.localAccounts(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(73), local.DefaultUserID)
}

// This fixture keeps the real SDK, account service, SQLite, selection store and
// HTTP relay; only the OAuth network destination is replaced with loopback TLS.
func TestMCPLoginProductionChain(t *testing.T) {
	// selection 与 restart 分支由 internal/mcpserver/pixiv/accounts 覆盖；这里只保留 CLI 组合根的四个不同结局。
	for _, scenario := range []string{"success", "helper-disconnect", "selection-failure", "shutdown"} {
		t.Run(scenario, func(t *testing.T) {
			dbPath, _ := useTempPaths(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			exchange := make(chan url.Values, 1)
			release := make(chan struct{})
			upstreamCanceled := make(chan struct{})
			var exchanges, connections atomic.Int32
			upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				exchanges.Add(1)
				if r.URL.Path != "/auth/token" {
					http.Error(w, "unexpected path", 400)
					return
				}
				if err := r.ParseForm(); err != nil {
					http.Error(w, "invalid form", 400)
					return
				}
				exchange <- r.Form
				select {
				case <-release:
				case <-r.Context().Done():
					close(upstreamCanceled)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"fixture-access","refresh_token":"fixture-refresh","expires_in":3600,"user":{"id":73,"name":"fixture"}}`)
			}))
			upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					connections.Add(1)
				}
				if state == http.StateClosed {
					connections.Add(-1)
				}
			}
			upstream.StartTLS()
			defer upstream.Close()
			transport := upstream.Client().Transport.(*http.Transport).Clone()
			transport.Proxy = nil
			fixtureURL, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			transport.TLSClientConfig.ServerName = fixtureURL.Hostname()
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != "oauth.secure.pixiv.net:443" {
					return nil, errors.New("fixture rejected unexpected network destination")
				}
				return (&net.Dialer{}).DialContext(ctx, network, fixtureURL.Host)
			}
			previousTransport := http.DefaultTransport
			http.DefaultTransport = transport
			defer func() { http.DefaultTransport = previousTransport; transport.CloseIdleConnections() }()
			a := newTestApp(nil, io.Discard, io.Discard)
			defer a.closeResources()
			oldServices := newCLIAccountServices
			loads := 0
			newCLIAccountServices = func(a app) (authcommands.AccountService, pixivaccount.LoginService, error) {
				loads++
				return oldServices(a)
			}
			defer func() { newCLIAccountServices = oldServices }()
			store := mcpauth.Store{Path: filepath.Join(t.TempDir(), "mcp-state.json")}
			_, err = store.Init(ctx, false)
			require.NoError(t, err)
			relayServer := httptest.NewUnstartedServer(nil)
			defer relayServer.Close()
			base := "http://" + relayServer.Listener.Addr().String() + "/fixture"
			ports, err := newCLIPixivSDKPorts(a)
			require.NoError(t, err)
			selection := mcpaccounts.Selection{Store: store, Load: ports.localAccounts}
			login, err := a.newMCPLogin(ctx, base, selection, nil)
			require.NoError(t, err)
			defer login.Close()
			protocol := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			handler, err := unifiedmcp.NewHTTPHandler(ctx, protocol, base, store, login)
			require.NoError(t, err)
			relayServer.Config.Handler = handler
			relayServer.Start()
			client := &http.Client{Transport: &http.Transport{}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			defer client.CloseIdleConnections()
			started, err := login.Start(false)
			require.NoError(t, err)
			page, err := client.Get(started.AuthorizationURL)
			require.NoError(t, err)
			page.Body.Close()
			require.Equal(t, 303, page.StatusCode)
			deepLink, err := url.Parse(page.Header.Get("Location"))
			require.NoError(t, err)
			origin, id, proof := deepLink.Query().Get("origin"), deepLink.Query().Get("session"), deepLink.Query().Get("access")
			start, err := client.Post(origin+"/start/"+id, "application/json", strings.NewReader(`{"proof":"`+proof+`"}`))
			require.NoError(t, err)
			var authorization struct {
				URL string `json:"authorization_url"`
			}
			require.NoError(t, json.NewDecoder(start.Body).Decode(&authorization))
			start.Body.Close()
			authorize, err := url.Parse(authorization.URL)
			require.NoError(t, err)
			require.NotEmpty(t, authorize.Query().Get("state"))
			callback := "pixiv://account/login?code=fixture&state=" + url.QueryEscape(authorize.Query().Get("state"))
			payload, _ := json.Marshal(map[string]string{"proof": proof, "callback_url": callback})
			type response struct {
				r   *http.Response
				err error
			}
			responses := make(chan response, 2)
			for range 2 {
				go func() {
					r, err := client.Post(origin+"/callback/"+id, "application/json", bytes.NewReader(payload))
					responses <- response{r, err}
				}()
			}
			var accepted *http.Response
			for range 2 {
				result := <-responses
				require.NoError(t, result.err)
				if result.r.StatusCode == 200 {
					require.Nil(t, accepted)
					accepted = result.r
				} else {
					require.Equal(t, 409, result.r.StatusCode)
					result.r.Body.Close()
				}
			}
			require.NotNil(t, accepted)
			defer accepted.Body.Close()
			var form url.Values
			select {
			case form = <-exchange:
			case <-time.After(5 * time.Second):
				t.Fatal("SDK exchange not reached")
			}
			digest := sha256.Sum256([]byte(form.Get("code_verifier")))
			require.Equal(t, authorize.Query().Get("code_challenge"), base64.RawURLEncoding.EncodeToString(digest[:]), "exchange must use the same PKCE session")
			require.Equal(t, "fixture", form.Get("code"))
			require.Equal(t, "authorization_code", form.Get("grant_type"))
			reused, err := login.Start(false)
			require.NoError(t, err)
			require.Equal(t, started.LoginID, reused.LoginID)
			if scenario == "restart" || scenario == "shutdown" {
				if scenario == "restart" {
					next, err := login.Start(true)
					require.NoError(t, err)
					require.NotEqual(t, started.LoginID, next.LoginID)
					require.Equal(t, "not_found", login.Status(started.LoginID).Status)
					require.Equal(t, 1, loads, "restart must reuse the account service instead of accumulating database handles")
				} else {
					login.Close()
					require.Equal(t, "failed", login.Status(started.LoginID).Status)
				}
				select {
				case <-upstreamCanceled:
				case <-time.After(5 * time.Second):
					t.Fatal("shutdown/restart did not cancel exchange")
				}
			} else {
				if scenario == "helper-disconnect" {
					accepted.Body.Close()
				}
				if scenario == "selection-failure" {
					require.NoError(t, os.Remove(store.Path))
					require.NoError(t, os.Mkdir(store.Path, 0o700))
				}
				if scenario == "selection-during-login" {
					require.NoError(t, store.SelectPixivUser(ctx, 42))
				}
				close(release)
				if scenario != "helper-disconnect" {
					resultPage, err := client.Get(accepted.Header.Get("X-Pixiv-Relay-Result-URL"))
					require.NoError(t, err)
					body, err := io.ReadAll(resultPage.Body)
					resultPage.Body.Close()
					require.NoError(t, err)
					if scenario == "selection-failure" {
						require.Equal(t, 400, resultPage.StatusCode)
						require.Contains(t, string(body), "Login failed")
					} else {
						require.Equal(t, 200, resultPage.StatusCode)
						require.Contains(t, string(body), "Login successful")
					}
				}
				require.Eventually(t, func() bool { s := login.Status(started.LoginID).Status; return s == "completed" || s == "failed" }, 5*time.Second, time.Millisecond)
				state := login.Status(started.LoginID)
				require.True(t, state.AccountSaved)
				require.NotNil(t, state.Account)
				require.Equal(t, int64(73), state.Account.UserID)
				require.Equal(t, scenario != "selection-failure", state.SelectionUpdated)
				if scenario == "selection-failure" {
					require.Equal(t, "failed", state.Status)
				} else {
					require.Equal(t, "completed", state.Status)
					saved, err := store.Read(ctx)
					require.NoError(t, err)
					require.Equal(t, int64(73), saved.SelectedPixivUserID)
				}
				serialized, err := json.Marshal(state)
				require.NoError(t, err)
				require.NotContains(t, string(serialized), "fixture-refresh")
				require.NotContains(t, string(serialized), proof)
			}
			db, err := database.Open(filepath.Dir(dbPath))
			require.NoError(t, err)
			defer db.Close()
			local, err := db.ListPixiv(ctx)
			require.NoError(t, err)
			if scenario == "shutdown" {
				require.Empty(t, local)
			} else {
				require.Len(t, local, 1)
				require.True(t, local[0].HasRefreshToken())
			}
			_, explicit, err := configapp.DefaultStore().ReadPixivDefaultUserID()
			require.NoError(t, err)
			require.False(t, explicit)
			require.Equal(t, int32(1), exchanges.Load(), "concurrent callbacks must exchange only once")
			require.Eventually(t, func() bool { return connections.Load() == 0 }, time.Second, time.Millisecond, "completed login must release its owned idle connection")
		})
	}
}

func TestMCPAnimationObservationsKeepSafeReasons(t *testing.T) {
	for _, tc := range []struct {
		name, source, reason string
	}{
		{"request", "request", "request cancelled"},
		{"transport", "transport", "transport interrupted"},
		{"subscription", "subscription", "subscription released"},
		{"service", "service", "service closed"},
		{"expiry", "expiry", "authorization expired"},
		{"authorization", "authorization", "authorization invalidated"},
		{"unknown source", "https://private.test/?token=secret", "tool failed"},
		{"failed without source", "", "tool failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := &syncBuffer{}
			logger, err := logging.New(output, "warn", "json")
			require.NoError(t, err)
			ctx := logging.WithCall(t.Context(), logger, "pixiv_gallery_animation", "2")
			logAnimation(ctx, ctx, mediaproxy.AnimationObservation{Stage: "animation_delivery", Duration: time.Second, Bytes: 17, Failed: tc.source == "", CancellationSource: tc.source})
			records := cliLogRecords(t, output.String(), "Animation animation_delivery")
			require.Len(t, records, 1)
			require.Equal(t, "WARN", records[0]["level"])
			require.Equal(t, tc.reason, records[0]["reason"])
			require.Equal(t, float64(17), records[0]["count"])
			require.Equal(t, "2", records[0]["call_id"])
			require.NotContains(t, output.String(), "token=secret")
		})
	}
}

// cliLogRecords 返回指定 operation 的 JSON 日志行。
func cliLogRecords(t *testing.T, output, prefix string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record), line)
		if message, _ := record["msg"].(string); strings.HasPrefix(message, prefix) {
			records = append(records, record)
		}
	}
	return records
}

// syncBuffer 让并发写入的日志在测试中可安全读取。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(data)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestMCPLocalRegistryWithoutHTTPState(t *testing.T) {
	_, configPath := useTempPaths(t)
	oldRun, oldPorts := runMCPStdio, newCLIPixivSDKPorts
	t.Cleanup(func() { runMCPStdio, newCLIPixivSDKPorts = oldRun, oldPorts })
	newCLIPixivSDKPorts = func(app) (pixivSDKPorts, error) {
		return pixivSDKPorts{watchReadSessions: func(context.Context) error { return errors.New("stdio must not start the HTTP read-session watcher") }}, nil
	}
	runMCPStdio = func(ctx context.Context, server *mcp.Server, _ io.Reader, _ io.Writer) error {
		ct, st := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, st, nil)
		require.NoError(t, err)
		defer ss.Close()
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "local", Version: "1"}, nil).Connect(ctx, ct, nil)
		require.NoError(t, err)
		defer cs.Close()
		listed, err := cs.ListTools(ctx, nil)
		require.NoError(t, err)
		require.NotEmpty(t, listed.Tools, "stdio must expose the local tool registry")
		return nil
	}
	var out, diagnostics bytes.Buffer
	require.Zero(t, RunContext(t.Context(), []string{"pixiv", "mcp"}, strings.NewReader(""), &out, &diagnostics), diagnostics.String())
	_, err := os.Stat(filepath.Join(filepath.Dir(configPath), "mcp-state.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Empty(t, out.String())
}
