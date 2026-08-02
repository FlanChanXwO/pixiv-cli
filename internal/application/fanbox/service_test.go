package fanbox

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/authdb"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func sessionOKRoundTripper() http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/html"}},
			Body:       io.NopCloser(strings.NewReader(`<html><head><meta name="metadata" content='{"context":{"user":{"userId":42,"name":"tester"}}}'></head></html>`)),
		}, nil
	})
}

func sessionFailRoundTripper() http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
}

func newTestService(t *testing.T) (*Service, *authdb.DB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	appDataDir := filepath.Join(home, constants.AppDataDirName)
	t.Cleanup(config.SetFilePathForTest(filepath.Join(appDataDir, "config.toml")))
	db, err := authdb.Open(appDataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return New(db, appDataDir), db
}

func TestImportSessionValidationFailureCreatesNoRecord(t *testing.T) {
	service, db := newTestService(t)
	service.OpenSessionFunc = func(value string) (*fanbox.Client, error) {
		return fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: value}, fanbox.Options{HTTPClient: &http.Client{Transport: sessionFailRoundTripper()}})
	}
	_, err := service.ImportSession(context.Background(), "session-canary", true)
	require.Error(t, err)
	require.Equal(t, sdk.CodeCredentialsExpired, sdk.CodeOf(err))
	accounts, err := db.ListFanbox(context.Background())
	require.NoError(t, err)
	require.Empty(t, accounts)
}

func TestImportSessionHappyPathStoresAndSelectsDefault(t *testing.T) {
	service, db := newTestService(t)
	service.OpenSessionFunc = func(value string) (*fanbox.Client, error) {
		return fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: value}, fanbox.Options{HTTPClient: &http.Client{Transport: sessionOKRoundTripper()}})
	}
	account, err := service.ImportSession(context.Background(), "session-canary", true)
	require.NoError(t, err)
	require.Equal(t, int64(42), account.UserID)
	require.True(t, account.Default)

	accounts, err := db.ListFanbox(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, "session-canary", string(accounts[0].SessionID))

	userID, ok, err := config.ReadFanboxDefaultUserID()
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(42), userID)
}

func TestUseAutoAndUseAccountToggleConfigDefault(t *testing.T) {
	service, _ := newTestService(t)
	service.OpenSessionFunc = func(value string) (*fanbox.Client, error) {
		return fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: value}, fanbox.Options{HTTPClient: &http.Client{Transport: sessionOKRoundTripper()}})
	}
	_, err := service.ImportSession(context.Background(), "session-abc", false)
	require.NoError(t, err)
	_, ok, err := config.ReadFanboxDefaultUserID()
	require.NoError(t, err)
	require.True(t, ok, "first imported account becomes default")

	require.NoError(t, service.UseAuto())
	_, ok, err = config.ReadFanboxDefaultUserID()
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, service.UseAccount(context.Background(), 42))
	userID, ok, err := config.ReadFanboxDefaultUserID()
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(42), userID)
}

func TestOpenClientUsesInjectedFunc(t *testing.T) {
	service, _ := newTestService(t)
	injected := &fanbox.Client{}
	service.OpenClientFunc = func(context.Context) (*fanbox.Client, error) {
		return injected, nil
	}
	client, err := service.OpenClient(context.Background())
	require.NoError(t, err)
	require.Same(t, injected, client)
}
