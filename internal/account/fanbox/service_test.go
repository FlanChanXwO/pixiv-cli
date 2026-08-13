package fanbox_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	accountfanbox "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
	database "github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
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

func newTestService(t *testing.T) (*accountfanbox.Service, *database.DB) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	appDataDir := filepath.Join(home, localstate.AppDataDirName)
	t.Cleanup(localstate.SetConfigFilePathForTest(filepath.Join(appDataDir, "config.toml")))
	db, err := database.Open(appDataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return accountfanbox.NewService(db, testFanboxDefaults{}), db
}

func TestImportSessionValidationFailureCreatesNoRecord(t *testing.T) {
	service, db := newTestService(t)
	service.OpenSessionFunc = func(value string) (*fanbox.Client, error) {
		return fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: value}, fanbox.Options{HTTPClient: &http.Client{Transport: sessionFailRoundTripper()}})
	}
	_, err := service.ImportSession(context.Background(), "session-canary", true)
	require.Error(t, err)
	require.Equal(t, sdk.CredentialsExpired, sdk.ReasonOf(err))
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
	require.Equal(t, "session-canary", string(accounts[0].SessionIDCopy()))

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

func TestRemoveAccountRejectsExplicitDefaultBeforeDatabaseMutation(t *testing.T) {
	service, db := newTestService(t)
	first := accountfanbox.New(42, "first", "", []byte("session"))
	first.ValidatedAt = 1
	require.NoError(t, db.SaveFanboxCredential(context.Background(), first))
	require.NoError(t, config.SetFanboxDefaultUserID(42))

	err := service.RemoveAccount(context.Background(), 42)
	require.Error(t, err)
	require.Contains(t, err.Error(), "auth use --auto")
	_, getErr := db.GetFanbox(context.Background(), 42)
	require.NoError(t, getErr)
	userID, ok, readErr := config.ReadFanboxDefaultUserID()
	require.NoError(t, readErr)
	require.True(t, ok)
	require.Equal(t, int64(42), userID)
}

func TestFanboxAutoDefaultMarksSmallestSortOrder(t *testing.T) {
	service, db := newTestService(t)
	second := accountfanbox.New(42, "second", "", []byte("session-2"))
	second.SortOrder, second.ValidatedAt = 2, 1
	first := accountfanbox.New(7, "first", "", []byte("session-1"))
	first.SortOrder, first.ValidatedAt = 1, 1
	require.NoError(t, db.SaveFanboxCredential(context.Background(), second))
	require.NoError(t, db.SaveFanboxCredential(context.Background(), first))

	accounts, err := service.ListAccounts(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.True(t, accounts[0].Default)
	require.False(t, accounts[1].Default)
	require.Equal(t, int64(7), accounts[0].UserID)
}

func TestFanboxDefaultConfigReadErrorIsReturnedBeforeRemoval(t *testing.T) {
	service, db := newTestService(t)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.Mkdir(configPath, 0o700))
	restore := localstate.SetConfigFilePathForTest(configPath)
	defer restore()
	first := accountfanbox.New(42, "first", "", []byte("session"))
	first.ValidatedAt = 1
	require.NoError(t, db.SaveFanboxCredential(context.Background(), first))

	err := service.RemoveAccount(context.Background(), 42)
	require.Error(t, err)
	_, getErr := db.GetFanbox(context.Background(), 42)
	require.NoError(t, getErr)
	require.False(t, errors.Is(err, accountfanbox.ErrNotFound))
}
