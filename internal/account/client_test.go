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
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
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

func TestSDKRefreshPersistsBeforeContent(t *testing.T) {
	store, err := account.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Username: "artist", Credential: credential.New([]byte("initial-refresh"))}))
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			fmt.Fprint(w, `{"access_token":"access","refresh_token":"rotated-refresh","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
			return
		}
		saved, err := store.GetPixiv(r.Context(), 42)
		if err != nil || saved.CredentialRevision != 2 || saved.Credential.Value() != "rotated-refresh" {
			t.Error("content request preceded credential persistence")
		}
		fmt.Fprint(w, `{"illust":{"id":101,"title":"fixture","type":"illust","create_date":"2026-01-01T00:00:00Z","user":{"id":7}}}`)
	}))
	defer api.Close()
	client, err := store.OpenSDKClient(t.Context(), 42, pixiv.Options{HTTPClient: oauthClient(api)})
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	_, err = client.Artwork(101).Get(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 42, client.UserID())
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
			current, err := store.GetPixiv(r.Context(), 42)
			if err != nil {
				t.Error(err)
				return
			}
			if err := store.RotatePixivCredentials(r.Context(), current, []byte("concurrent")); err != nil {
				t.Error(err)
			}
		} else if token != "concurrent" {
			t.Error("conflict did not reload stored credential")
		}
		fmt.Fprint(w, `{"access_token":"access","refresh_token":"fresh","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
	}))
	defer api.Close()
	client, err := store.OpenAccountClient(t.Context(), 42, legacysdk.Options{HTTPClient: oauthClient(api)})
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
	snapshot, err := reads.Open(t.Context(), 42, "network", legacysdk.Options{HTTPClient: oauthClient(api)})
	require.NoError(t, err)
	current, err := store.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.NoError(t, store.RotatePixivCredentials(t.Context(), current, []byte("changed")))
	_, err = snapshot.Client.CurrentUser(t.Context(), legacysdk.CurrentUserRequest{})
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
	client, err := store.OpenAccountClient(t.Context(), 42, legacysdk.Options{HTTPClient: oauthClient(api)})
	require.Error(t, err)
	require.Nil(t, client)
}

func TestReadCacheReusesAndReloadsByRevision(t *testing.T) {
	store, err := account.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Credential: credential.New([]byte("initial"))}))
	var requests atomic.Int32
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 2 && r.FormValue("refresh_token") != "external" {
			t.Error("cache reload used stale credential")
		}
		fmt.Fprint(w, `{"access_token":"access","refresh_token":"fresh","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
	}))
	defer api.Close()
	reads := account.NewReadSessions(store, account.NewGate())
	defer reads.Close()
	options := legacysdk.Options{HTTPClient: oauthClient(api)}
	first, err := reads.Open(t.Context(), 42, "network", options)
	require.NoError(t, err)
	cached, err := reads.Open(t.Context(), 42, "network", options)
	require.NoError(t, err)
	require.Equal(t, first.Version, cached.Version)
	require.EqualValues(t, 1, requests.Load())
	current, err := store.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.NoError(t, store.RotatePixivCredentials(t.Context(), current, []byte("external")))
	latest, err := reads.Open(t.Context(), 42, "network", options)
	require.NoError(t, err)
	require.NotEqual(t, first.Version, latest.Version)
	require.EqualValues(t, 4, latest.Revision)
	require.EqualValues(t, 2, requests.Load())
	select {
	case <-first.Invalidated:
	default:
		t.Error("replaced generation was not cancelled")
	}
}

func TestRecreatedAccountInvalidatesReadLease(t *testing.T) {
	store, err := account.Open(t.Context(), t.TempDir())
	require.NoError(t, err)
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Credential: credential.New([]byte("old"))}))
	var requests atomic.Int32
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{"access_token":"access","refresh_token":"fresh","expires_in":3600,"user":{"id":"42","name":"artist"}}`)
	}))
	defer api.Close()
	reads := account.NewReadSessions(store, account.NewGate())
	defer reads.Close()
	lease, err := reads.Open(t.Context(), 42, "network", legacysdk.Options{HTTPClient: oauthClient(api)})
	require.NoError(t, err)
	require.NoError(t, store.RemovePixiv(t.Context(), 42))
	require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Credential: credential.New([]byte("replacement"))}))
	current, err := store.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.NoError(t, store.RotatePixivCredentials(t.Context(), current, []byte("replacement-rotated")))
	_, err = lease.Client.CurrentUser(t.Context(), legacysdk.CurrentUserRequest{})
	require.Error(t, err)
	require.EqualValues(t, 1, requests.Load(), "stale lease reached the upstream")
}
