package bootstrap

import (
	"errors"
	"testing"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	"github.com/stretchr/testify/require"
)

// isolatedRuntime 在隔离 HOME 下构造 Runtime，避免测试写宿主目录。
func isolatedRuntime(t *testing.T) *Runtime {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	runtime, err := NewRuntime(RuntimeOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	return runtime
}

func TestRuntimeSDKRejectsMalformedExplicitProxyAtConstruction(t *testing.T) {
	clearRuntimeEnvironment(t)
	invalidProxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"
	runtime := isolatedRuntime(t)

	client, err := runtime.SDK.Client(pixivapp.SDKClientRequest{HTTPSProxyOverride: &invalidProxy})

	require.Equal(t, pixivapp.ClientSet{}, client)
	require.Error(t, err)
	require.ErrorIs(t, err, uri.ErrInvalidProxy)
	require.Contains(t, err.Error(), "invalid proxy")
	for current := err; current != nil; current = errors.Unwrap(current) {
		for _, secret := range []string{"proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret"} {
			require.NotContains(t, current.Error(), secret)
		}
	}
}

func TestRuntimeSDKOpenOperationRejectsMissingRequestedAccountBeforeOAuth(t *testing.T) {
	clearRuntimeEnvironment(t)
	runtime := isolatedRuntime(t)

	client, err := runtime.SDK.OpenOperation(t.Context(), pixivapp.SDKClientRequest{UserID: 99})

	require.Equal(t, pixivapp.ClientSet{}, client)
	require.Error(t, err)
	require.ErrorContains(t, err, "select pixiv account")
	require.ErrorContains(t, err, "not found")
}

func TestRuntimeSDKOpenOperationRejectsNoAccounts(t *testing.T) {
	clearRuntimeEnvironment(t)
	runtime := isolatedRuntime(t)

	client, err := runtime.SDK.Client(pixivapp.SDKClientRequest{})

	require.Equal(t, pixivapp.ClientSet{}, client)
	require.Error(t, err)
	require.ErrorContains(t, err, "no pixiv account is authenticated")
}

func TestRuntimeConfiguresDownloadFactoryWithSDKClient(t *testing.T) {
	clearRuntimeEnvironment(t)
	runtime := isolatedRuntime(t)
	require.NotNil(t, runtime.Download.NewManager)
}
