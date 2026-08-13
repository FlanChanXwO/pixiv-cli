package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type resourceLoadState struct {
	runtime  bool
	database bool
	pixiv    bool
	account  bool
	login    bool
	sdk      bool
	download bool
	fanbox   bool
	update   bool
}

func resourceLoadStateFrom(resources *runResources) resourceLoadState {
	return resourceLoadState{
		runtime:  resources.runtimeLoaded,
		database: resources.databaseLoaded,
		pixiv:    resources.pixivLoaded,
		account:  resources.accountLoaded,
		login:    resources.loginLoaded,
		sdk:      resources.sdkLoaded,
		download: resources.downloadLoaded,
		fanbox:   resources.fanboxLoaded,
		update:   resources.updateLoaded,
	}
}

func TestCommandRequirementsCoverStartupMatrix(t *testing.T) {
	a := app{
		in:             strings.NewReader(""),
		out:            io.Discard,
		errOut:         io.Discard,
		resourcesState: &resourcesState{},
		debugState:     &debugState{},
	}
	root := a.newRootCommand()
	t.Cleanup(func() { requirements.Clear(root) })

	for _, test := range []struct {
		name string
		args []string
		want requirements.Execution
	}{
		{name: "auth export", args: []string{"auth", "export"}, want: requirements.AuthExport()},
		{name: "hidden callback", args: []string{"auth", internalURLCallbackCommand}, want: requirements.HiddenCallback()},
		{name: "version", args: []string{"version"}, want: requirements.Version()},
		{name: "update", args: []string{"update"}, want: requirements.UpdateCommand()},
		{name: "config path", args: []string{"config", "path"}, want: requirements.ConfigPath()},
		{name: "auth token import", args: []string{"auth", "import"}, want: requirements.AuthAccount()},
		{name: "auth login", args: []string{"auth", "login"}, want: requirements.AuthLogin()},
		{name: "pixiv data", args: []string{"search"}, want: requirements.PixivData()},
		{name: "download", args: []string{"download"}, want: requirements.DownloadCommand()},
		{name: "fanbox auth", args: []string{"fanbox", "auth", "import"}, want: requirements.FanboxAuth()},
		{name: "fanbox data", args: []string{"fanbox", "posts"}, want: requirements.FanboxData()},
		{name: "pixiv mcp", args: []string{"mcp"}, want: requirements.PixivMCP()},
		{name: "fanbox mcp", args: []string{"fanbox", "mcp"}, want: requirements.FanboxMCP()},
	} {
		t.Run(test.name, func(t *testing.T) {
			command, _, err := root.Find(test.args)
			require.NoError(t, err)
			require.NotNil(t, command)
			assert.Equal(t, test.want, requirements.For(command))
		})
	}
}

func TestRunResourcesPrepareConstructsOnlyDeclaredNodes(t *testing.T) {
	useTempPaths(t)

	for _, test := range []struct {
		name string
		need requirements.Resources
		want resourceLoadState
	}{
		{name: "none"},
		{name: "snapshot", need: requirements.Resources{ConfigSnapshot: true}, want: resourceLoadState{runtime: true}},
		{name: "account", need: requirements.Resources{PixivAccount: true}, want: resourceLoadState{database: true, pixiv: true, account: true}},
		{name: "login", need: requirements.Resources{PixivLogin: true}, want: resourceLoadState{database: true, pixiv: true, login: true}},
		{name: "sdk", need: requirements.Resources{PixivSDK: true}, want: resourceLoadState{database: true, pixiv: true, sdk: true}},
		{name: "download", need: requirements.Resources{Download: true}, want: resourceLoadState{download: true}},
		{name: "fanbox", need: requirements.Resources{Fanbox: true}, want: resourceLoadState{database: true, fanbox: true}},
		{name: "update", need: requirements.Resources{Update: true}, want: resourceLoadState{update: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			resources := &runResources{}
			require.NoError(t, resources.prepare(test.need))
			assert.Equal(t, test.want, resourceLoadStateFrom(resources))
			require.NoError(t, resources.close())
		})
	}
}

type startupSideEffectProbe struct {
	cleanupCalls  int
	handlerCalls  int
	resourceCalls int
}

func installStartupSideEffectProbe(t *testing.T) (*startupSideEffectProbe, string) {
	t.Helper()
	_, configPath := useTempPaths(t)
	oldCleanup := cleanupPendingWindowsUpdate
	oldSupported := automaticPersistentHandlerSupported
	oldEnsure := ensureURLSchemeRelay
	oldResources := newCLIRunResources
	oldRuntime := loadCLIRuntimeConfig
	probe := &startupSideEffectProbe{}
	cleanupPendingWindowsUpdate = func() error {
		probe.cleanupCalls++
		return nil
	}
	automaticPersistentHandlerSupported = func() bool { return true }
	ensureURLSchemeRelay = func(context.Context) error {
		probe.handlerCalls++
		return nil
	}
	newCLIRunResources = func() (*runResources, error) {
		probe.resourceCalls++
		return &runResources{}, nil
	}
	loadCLIRuntimeConfig = func() (config.RuntimeConfig, error) {
		t.Fatal("invalid auth import input must not load runtime configuration")
		return config.RuntimeConfig{}, nil
	}
	t.Cleanup(func() {
		cleanupPendingWindowsUpdate = oldCleanup
		automaticPersistentHandlerSupported = oldSupported
		ensureURLSchemeRelay = oldEnsure
		newCLIRunResources = oldResources
		loadCLIRuntimeConfig = oldRuntime
	})
	return probe, configPath
}

func (p *startupSideEffectProbe) assertNone(t *testing.T, configPath string) {
	t.Helper()
	assert.Zero(t, p.cleanupCalls)
	assert.Zero(t, p.handlerCalls)
	assert.Zero(t, p.resourceCalls)
	_, err := os.Stat(configPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunInputValidationFailureSkipsStartupAndResourceSideEffects(t *testing.T) {
	probe, configPath := installStartupSideEffectProbe(t)

	const secret = "malformed-bundle-secret"
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "import"}, strings.NewReader("{\"token\":\""+secret+"\""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "invalid auth export bundle JSON")
	assert.NotContains(t, stderr.String(), secret)
	probe.assertNone(t, configPath)
}

func TestRunRootHelpSkipsStartupAndResourceSideEffects(t *testing.T) {
	probe, configPath := installStartupSideEffectProbe(t)
	var stdout, stderr bytes.Buffer

	code := Run([]string{"pixiv"}, strings.NewReader(""), &stdout, &stderr)

	require.Zero(t, code, stderr.String())
	assert.Contains(t, stdout.String(), "Usage:")
	assert.Empty(t, stderr.String())
	probe.assertNone(t, configPath)
}

func TestRunResourcesCloseReversesClosersAndJoinsErrors(t *testing.T) {
	firstErr := errors.New("first close failed")
	secondErr := errors.New("second close failed")
	var order []string
	resources := &runResources{closers: []func() error{
		func() error {
			order = append(order, "first")
			return firstErr
		},
		func() error {
			order = append(order, "second")
			return secondErr
		},
		func() error {
			order = append(order, "third")
			return nil
		},
	}}

	err := resources.close()

	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, secondErr)
	assert.Equal(t, []string{"third", "second", "first"}, order)
	require.ErrorIs(t, resources.close(), firstErr)
	assert.Equal(t, []string{"third", "second", "first"}, order)
}

func TestRunJoinsCommandAndResourceCloseErrors(t *testing.T) {
	useTempPaths(t)
	commandErr := errors.New("update execution failed")
	closeErr := errors.New("database close failed")
	oldResources := newCLIRunResources
	oldLoad := loadCLIRuntimeConfig
	oldCoordinator := newUpdateCommandCoordinator
	oldCleanup := cleanupPendingWindowsUpdate
	newCLIRunResources = func() (*runResources, error) {
		return &runResources{closers: []func() error{func() error { return closeErr }}}, nil
	}
	loadCLIRuntimeConfig = func() (config.RuntimeConfig, error) { return config.RuntimeConfig{}, nil }
	newUpdateCommandCoordinator = func(string, io.Writer, io.Writer) (*update.UpdateCoordinator, error) {
		return nil, commandErr
	}
	cleanupPendingWindowsUpdate = func() error { return nil }
	t.Cleanup(func() {
		newCLIRunResources = oldResources
		loadCLIRuntimeConfig = oldLoad
		newUpdateCommandCoordinator = oldCoordinator
		cleanupPendingWindowsUpdate = oldCleanup
	})

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "update", "--check"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 1, code)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), commandErr.Error())
	assert.Contains(t, stderr.String(), closeErr.Error())
}
