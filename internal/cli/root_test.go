package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	pixivdeps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth"
	mcpcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/mcp"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestRunUnknownCommandReturnsError(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "wat"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), `unknown command "wat"`)
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

// cobraBuiltInCommands are generated by Cobra rather than declared by a
// command owner. They never consume our stdin contract.
var cobraBuiltInCommands = map[string]struct{}{
	"_help":      {},
	"completion": {},
	"help":       {},
}

func TestEveryRunnableCommandDeclaresAnInputContract(t *testing.T) {
	root := newTestRootCommand(t)

	missing := commandsWithoutInputContract(root)
	if len(missing) > 0 {
		t.Fatalf("runnable commands without a declared input contract:\n  %s", strings.Join(missing, "\n  "))
	}
}

func TestDeclaredInputContractsUseAKnownCodec(t *testing.T) {
	root := newTestRootCommand(t)

	known := map[pipeline.Codec]struct{}{
		pipeline.NoInput:      {},
		pipeline.TextValue:    {},
		pipeline.TextOrRecord: {},
	}
	unknown := make([]string, 0)
	forEachCommand(root, func(command *cobra.Command) {
		annotation, declared := command.Annotations[pipeline.AnnotationKey]
		if !declared {
			return
		}
		codec, _, ok := strings.Cut(annotation, ":")
		if !ok {
			unknown = append(unknown, command.CommandPath()+" has a malformed contract "+annotation)
			return
		}
		if _, valid := known[pipeline.Codec(codec)]; !valid {
			unknown = append(unknown, command.CommandPath()+" declares unknown codec "+codec)
		}
	})

	sort.Strings(unknown)
	if len(unknown) > 0 {
		t.Fatalf("invalid declared input contracts:\n  %s", strings.Join(unknown, "\n  "))
	}
}

func TestProtocolOwningCommandsReserveStdin(t *testing.T) {
	root := newTestRootCommand(t)

	for _, args := range [][]string{{"mcp"}, {"fanbox", "mcp"}} {
		command, _, err := root.Find(args)
		require.NoError(t, err)
		annotation := command.Annotations[pipeline.AnnotationKey]
		codec, _, _ := strings.Cut(annotation, ":")
		assert.Equal(t, pipeline.NoInput, pipeline.Codec(codec), command.CommandPath())
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

func commandsWithoutInputContract(root *cobra.Command) []string {
	missing := make([]string, 0)
	forEachCommand(root, func(command *cobra.Command) {
		if command == root || !command.Runnable() {
			return
		}
		if _, builtIn := cobraBuiltInCommands[command.Name()]; builtIn {
			return
		}
		if _, declared := command.Annotations[pipeline.AnnotationKey]; !declared {
			missing = append(missing, command.CommandPath())
		}
	})
	sort.Strings(missing)
	return missing
}

func forEachCommand(command *cobra.Command, visit func(*cobra.Command)) {
	visit(command)
	for _, child := range command.Commands() {
		forEachCommand(child, visit)
	}
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
