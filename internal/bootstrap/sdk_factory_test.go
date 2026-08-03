package bootstrap

import (
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	"github.com/stretchr/testify/require"
)

// isolatedServices 在隔离 HOME 下构造 Services，避免测试写宿主目录。
func isolatedServices(t *testing.T) application.Services {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	services := NewServices()
	t.Cleanup(CloseServices)
	return services
}

func TestNewServicesSDKRejectsMalformedExplicitProxyAtConstruction(t *testing.T) {
	clearRuntimeEnvironment(t)
	invalidProxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"
	services := isolatedServices(t)

	client, err := services.SDK.Client(application.SDKClientRequest{HTTPSProxyOverride: &invalidProxy})

	require.Nil(t, client)
	require.Error(t, err)
	require.ErrorIs(t, err, uri.ErrInvalidProxy)
	require.Contains(t, err.Error(), "invalid proxy")
	for current := err; current != nil; current = errors.Unwrap(current) {
		for _, secret := range []string{"proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret"} {
			require.NotContains(t, current.Error(), secret)
		}
	}
}

func TestNewServicesSDKOpenOperationRejectsMissingRequestedAccountBeforeOAuth(t *testing.T) {
	clearRuntimeEnvironment(t)
	services := isolatedServices(t)

	client, err := services.SDK.OpenOperation(t.Context(), application.SDKClientRequest{UserID: 99})

	require.Nil(t, client)
	require.Error(t, err)
	require.ErrorContains(t, err, "select pixiv account")
	require.ErrorContains(t, err, "not found")
}

func TestNewServicesSDKOpenOperationRejectsNoAccounts(t *testing.T) {
	clearRuntimeEnvironment(t)
	services := isolatedServices(t)

	client, err := services.SDK.Client(application.SDKClientRequest{})

	require.Nil(t, client)
	require.Error(t, err)
	require.ErrorContains(t, err, "no pixiv account is authenticated")
}

func TestNewServicesConfiguresDownloadFactoryWithSDKClient(t *testing.T) {
	clearRuntimeEnvironment(t)
	services := isolatedServices(t)
	require.NotNil(t, services.Download.NewManager)
}
