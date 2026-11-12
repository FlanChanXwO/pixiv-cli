package account_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func TestRotationPersistsNewTokenBeforeReturningClient(t *testing.T) {
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("refresh_token") != "initial-refresh" {
			t.Error("OAuth request did not use the stored credential")
		}
		fmt.Fprint(w, `{"access_token":"access","refresh_token":"rotated-refresh","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
	}))
	defer api.Close()
	httpClient := api.Client()
	transport := httpClient.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
	}
	directory := t.TempDir()
	db, err := database.Open(directory)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SavePixivCredential(t.Context(), account.New(42, "artist", []byte("initial-refresh"))))
	client, err := account.NewService(db, nil).OpenAccountClient(t.Context(), 42, legacysdk.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	require.EqualValues(t, 42, client.UserID())
	require.NoError(t, db.Close())
	db, err = database.Open(directory)
	require.NoError(t, err)
	saved, err := db.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.Equal(t, "rotated-refresh", string(saved.RefreshTokenCopy()))
	require.EqualValues(t, 2, saved.CredentialRevision)
}

func TestSDKRefreshPersistsBeforeContent(t *testing.T) {
	db, err := database.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SavePixivCredential(t.Context(), account.New(42, "artist", []byte("initial-refresh"))))
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			if r.FormValue("refresh_token") != "initial-refresh" {
				t.Error("OAuth did not use the saved credential")
			}
			fmt.Fprint(w, `{"access_token":"access","refresh_token":"rotated-refresh","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
			return
		}
		saved, err := db.GetPixiv(r.Context(), 42)
		if err != nil || saved.CredentialRevision != 2 || saved.Credential.Value() != "rotated-refresh" {
			t.Error("content request preceded credential persistence")
		}
		fmt.Fprint(w, `{"illust":{"id":101,"title":"fixture","type":"illust","create_date":"2026-01-01T00:00:00Z","user":{"id":7}}}`)
	}))
	defer api.Close()
	httpClient := api.Client()
	transport := httpClient.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
	}
	client, err := account.NewService(db, nil).OpenSDKClient(t.Context(), 42, pixiv.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	_, err = client.Artwork(101).Get(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 42, client.UserID())
}
