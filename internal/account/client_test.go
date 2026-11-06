package account_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

func oauthClient(api *httptest.Server) *http.Client {
	client := api.Client()
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
	}
	return client
}

func TestRotationPersistsNewTokenBeforeReturningClient(t *testing.T) {
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("refresh_token") != "initial-refresh" {
			t.Error("OAuth did not use stored credential")
		}
		fmt.Fprint(w, `{"access_token":"access","refresh_token":"rotated-refresh","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
	}))
	defer api.Close()
	directory := t.TempDir()
	store, err := account.Open(t.Context(), directory)
	require.NoError(t, err)
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Username: "artist", Credential: credential.New([]byte("initial-refresh"))}))
	client, err := store.OpenAccountClient(t.Context(), 42, pixiv.Options{HTTPClient: oauthClient(api)})
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	require.EqualValues(t, 42, client.UserID())
	reopened, err := account.Open(t.Context(), directory)
	require.NoError(t, err)
	saved, err := reopened.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.Equal(t, "rotated-refresh", saved.Credential.Value())
	require.EqualValues(t, 2, saved.CredentialRevision)
}

func TestRotationConflictUsesLatestCredential(t *testing.T) {
	store, err := account.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Credential: credential.New([]byte("initial"))}))
	var requests atomic.Int32
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.FormValue("refresh_token")
		if requests.Add(1) == 1 {
			if token != "initial" {
				t.Error("unexpected first token")
			}
			if err := store.RotatePixivCredentials(r.Context(), 42, 1, []byte("concurrent")); err != nil {
				t.Error(err)
			}
		} else if token != "concurrent" {
			t.Error("conflict did not reload stored credential")
		}
		fmt.Fprint(w, `{"access_token":"access","refresh_token":"fresh","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
	}))
	defer api.Close()
	client, err := store.OpenAccountClient(t.Context(), 42, pixiv.Options{HTTPClient: oauthClient(api)})
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	saved, err := store.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.EqualValues(t, 3, saved.CredentialRevision)
	require.Equal(t, "fresh", saved.Credential.Value())
	require.EqualValues(t, 2, requests.Load())
}

func TestReadLeaseChecksRevisionBeforeRequest(t *testing.T) {
	store, err := account.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Credential: credential.New([]byte("initial"))}))
	var contentRequests atomic.Int32
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("refresh_token") != "" {
			fmt.Fprint(w, `{"access_token":"access","refresh_token":"fresh","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
			return
		}
		contentRequests.Add(1)
		http.Error(w, "unexpected stale lease request", http.StatusBadRequest)
	}))
	defer api.Close()
	reads := account.NewReadSessions(store, account.NewGate())
	defer reads.Close()
	snapshot, err := reads.Open(t.Context(), 42, "network", pixiv.Options{HTTPClient: oauthClient(api)})
	require.NoError(t, err)
	require.NoError(t, store.RotatePixivCredentials(t.Context(), 42, 2, []byte("changed")))
	_, err = snapshot.Client.CurrentUser(t.Context(), pixiv.CurrentUserRequest{})
	require.Error(t, err)
	require.Zero(t, contentRequests.Load())
	select {
	case <-snapshot.Invalidated:
	default:
		t.Error("stale read generation remained live")
	}
}

func TestPersistenceFailureDoesNotReturnClient(t *testing.T) {
	directory := t.TempDir()
	store, err := account.Open(t.Context(), directory)
	require.NoError(t, err)
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Credential: credential.New([]byte("initial"))}))
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(directory, "auth.json")
		if err := os.Rename(path, path+".saved"); err != nil {
			t.Error(err)
		}
		if err := os.Mkdir(path, 0700); err != nil {
			t.Error(err)
		}
		fmt.Fprint(w, `{"access_token":"access","refresh_token":"uncommitted","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
	}))
	defer api.Close()
	client, err := store.OpenAccountClient(t.Context(), 42, pixiv.Options{HTTPClient: oauthClient(api)})
	require.Error(t, err)
	require.Nil(t, client)
}
