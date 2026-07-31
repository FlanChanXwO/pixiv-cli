package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/stretchr/testify/require"
)

func TestNewServicesSDKKeepsConfiguredProxyDynamicUntilOperation(t *testing.T) {
	clearRuntimeEnvironment(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	t.Cleanup(config.SetFilePathForTest(configPath))
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = \"\"\n")))

	client, err := NewServices().SDK.Client(application.SDKClientRequest{})
	require.NoError(t, err)

	invalidProxy := "http://bootstrap-proxy.invalid/path-%zz"
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = "+fmt.Sprintf("%q", invalidProxy)+"\n")))
	_, err = client.ParseResourceRef("https://i.pximg.net/img-original/a.jpg")

	var typed *sdk.Error
	require.ErrorAs(t, err, &typed)
	require.Equal(t, sdk.OperationParseResourceRef, typed.Operation)
	require.Equal(t, sdk.LocalStateKindInvalidProxy, typed.LocalStateKind)
	require.ErrorIs(t, err, sdk.ErrInvalidArgument)
	require.NotContains(t, err.Error(), invalidProxy)
	unwrapped := errors.Unwrap(err)
	require.NotNil(t, unwrapped)
	require.NotContains(t, unwrapped.Error(), invalidProxy)
}

func TestNewServicesSDKExplicitEmptyProxyOverridesConfiguredProxy(t *testing.T) {
	clearRuntimeEnvironment(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	t.Cleanup(config.SetFilePathForTest(configPath))
	invalidProxy := "http://bootstrap-proxy.invalid/path-%zz"
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[network]\nhttps_proxy = "+fmt.Sprintf("%q", invalidProxy)+"\n")))
	emptyProxy := ""

	client, err := NewServices().SDK.Client(application.SDKClientRequest{HTTPSProxyOverride: &emptyProxy})
	require.NoError(t, err)
	ref, err := client.ParseResourceRef("https://i.pximg.net/img-original/a.jpg")

	require.NoError(t, err)
	require.Equal(t, "https://i.pximg.net/img-original/a.jpg", ref.URL)
}

func TestNewServicesSDKRejectsMalformedExplicitProxyAtConstruction(t *testing.T) {
	invalidProxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"

	client, err := NewServices().SDK.Client(application.SDKClientRequest{HTTPSProxyOverride: &invalidProxy})

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
	t.Setenv("PIXIV_REFRESH_TOKEN", "")
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	authPath := filepath.Join(dir, "auth.json")
	t.Cleanup(config.SetFilePathForTest(configPath))
	require.NoError(t, auth.SaveAuthStore(authPath, auth.AuthStore{
		DefaultUserID: 7,
		Accounts:      []auth.Account{{UserID: 7, Username: "available", RefreshToken: "unused-token"}},
	}))

	client, err := NewServices().SDK.OpenOperation(context.Background(), application.SDKClientRequest{
		UserID:       99,
		AuthFilePath: authPath,
	})

	require.Nil(t, client)
	var typed *sdk.Error
	require.ErrorAs(t, err, &typed)
	require.ErrorIs(t, err, sdk.ErrInvalidArgument)
	require.Equal(t, sdk.OperationSnapshot, typed.Operation)
	require.EqualValues(t, 99, typed.UserID)
	require.Empty(t, typed.Backend)
}

func TestNewServicesSDKRejectsCookieRefreshTokenAtConstructionWithoutEcho(t *testing.T) {
	cookie := "refresh_token=bootstrap-cookie-value"

	client, err := NewServices().SDK.Client(application.SDKClientRequest{RefreshToken: cookie})

	require.Nil(t, client)
	var typed *sdk.Error
	require.ErrorAs(t, err, &typed)
	require.ErrorIs(t, err, sdk.ErrInvalidArgument)
	require.NotContains(t, err.Error(), cookie)
	unwrapped := errors.Unwrap(err)
	require.NotNil(t, unwrapped)
	require.NotContains(t, unwrapped.Error(), cookie)
}

func TestNewServicesSDKExplicitAuthPathOverridesGlobalPath(t *testing.T) {
	clearRuntimeEnvironment(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	globalAuthPath := filepath.Join(dir, "global-auth.json")
	explicitAuthPath := filepath.Join(dir, "explicit-auth.json")
	t.Cleanup(config.SetFilePathForTest(configPath))
	t.Cleanup(auth.SetAuthFilePathForTest(globalAuthPath))
	require.NoError(t, auth.SaveAuthStore(globalAuthPath, auth.AuthStore{
		DefaultUserID: 7,
		Accounts:      []auth.Account{{UserID: 7, Username: "global", RefreshToken: "global-token"}},
	}))
	require.NoError(t, auth.SaveAuthStore(explicitAuthPath, auth.AuthStore{
		DefaultUserID: 8,
		Accounts:      []auth.Account{{UserID: 8, Username: "explicit", RefreshToken: "explicit-token"}},
	}))

	client, err := NewServices().SDK.Client(application.SDKClientRequest{AuthFilePath: explicitAuthPath})
	require.NoError(t, err)
	accounts, err := client.ListAccounts()

	require.NoError(t, err)
	require.EqualValues(t, 8, accounts.DefaultUserID)
	require.Equal(t, []sdk.Account{{UserID: 8, Username: "explicit", Default: true, HasToken: true}}, accounts.Accounts)
}
