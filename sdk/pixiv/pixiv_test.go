package pixiv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/internal/cursor"
	"github.com/stretchr/testify/require"
)

type refreshFixture struct {
	CredentialStore
	mode, failPath              string
	newer                       Credentials
	saves, oauthCalls, appCalls atomic.Int32
	access                      atomic.Value
}

func (f *refreshFixture) CompareAndSwap(ctx context.Context, revision int64, next Credentials) (bool, error) {
	f.saves.Add(1)
	if f.failPath != "" {
		return false, os.WriteFile(f.failPath, []byte("{}"), 0600)
	}
	return f.CredentialStore.CompareAndSwap(ctx, revision, next)
}

func (f *refreshFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == api.PathOAuthToken {
		f.oauthCalls.Add(1)
		if f.mode == "blank_access" {
			fmt.Fprint(w, `{"access_token":" ","refresh_token":"rotated-fixture","user":{"id":11}}`)
			return
		}
		if f.mode == "conflict" || f.mode == "oauth_conflict" {
			saved, err := f.CredentialStore.CompareAndSwap(r.Context(), 1, f.newer)
			if err != nil || !saved {
				http.Error(w, "fixture save failed", 500)
				return
			}
			if f.mode == "oauth_conflict" {
				http.Error(w, "rotated elsewhere", 400)
				return
			}
		}
		fmt.Fprint(w, `{"access_token":"new-fixture","refresh_token":"rotated-fixture","expires_in":3600,"user":{"id":"11","name":"fixture"}}`)
		return
	}
	f.appCalls.Add(1)
	current, err := f.Load(r.Context())
	if err != nil || current.Revision != 2 {
		http.Error(w, "credentials were not saved before content", 500)
		return
	}
	f.access.Store(r.Header.Get("Authorization"))
	fmt.Fprint(w, `{"ok":true}`)
}

func fixtureHTTP(server *httptest.Server) *http.Client {
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = server.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	return client
}

func TestCredentialRefresh(t *testing.T) {
	for _, mode := range []string{"expired", "already_saved", "conflict", "oauth_conflict", "save_error", "blank_access"} {
		t.Run(mode, func(t *testing.T) {
			initial := NewCredentials("refresh-fixture")
			initial.SetTokens("expired-fixture", "refresh-fixture")
			initial.UserID, initial.ExpiresAt = 11, time.Now().Add(-time.Hour)
			store, err := NewMemoryCredentials(initial)
			require.NoError(t, err)
			newer := initial
			newer.SetTokens("saved-fixture", "saved-refresh-fixture")
			newer.Revision, newer.ExpiresAt = 2, time.Now().Add(time.Hour)
			fixture := &refreshFixture{CredentialStore: store, mode: mode, newer: newer}
			server := httptest.NewTLSServer(fixture)
			defer server.Close()
			client, err := New(fixture, Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			if mode == "already_saved" {
				saved, err := store.CompareAndSwap(t.Context(), 1, newer)
				require.NoError(t, err)
				require.True(t, saved)
			}
			if mode == "save_error" {
				fixture.failPath = filepath.Join(t.TempDir(), "missing", "auth.json")
			}
			var result map[string]any
			err = client.getJSON(t.Context(), api.PathUserDetail, nil, &result)
			assertRefreshed(t, fixture, err)
		})
	}
}

func assertRefreshed(t *testing.T, fixture *refreshFixture, err error) {
	t.Helper()
	mode := fixture.mode
	if mode == "blank_access" {
		require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		require.Zero(t, fixture.saves.Load())
		require.Zero(t, fixture.appCalls.Load())
		return
	}
	if mode == "save_error" {
		require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
		require.ErrorIs(t, err, os.ErrNotExist)
		require.Zero(t, fixture.appCalls.Load())
		return
	}
	require.NoError(t, err)
	require.EqualValues(t, 1, fixture.appCalls.Load())
	expected := "Bearer saved-fixture"
	if mode == "expired" {
		expected = "Bearer new-fixture"
	}
	require.Equal(t, expected, fixture.access.Load())
	saved, err := fixture.Load(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 2, saved.Revision)
	if mode == "expired" {
		require.Equal(t, "rotated-fixture", saved.RefreshToken.Value())
		require.EqualValues(t, 1, fixture.saves.Load())
	}
	if mode == "already_saved" {
		require.Zero(t, fixture.oauthCalls.Load())
		require.Zero(t, fixture.saves.Load())
	}
}

func TestCursorQueryBinding(t *testing.T) {
	original := cursor.Binding{Product: "pixiv", Operation: "search", Account: "11", Query: url.Values{"word": {"private-search"}}}
	token, err := cursor.Encode(original, url.Values{"offset": {"30"}})
	require.NoError(t, err)
	changed := original
	changed.Query = url.Values{"word": {"different"}}
	_, err = cursor.Decode(token, changed)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	changed = original
	changed.Account = "12"
	_, err = cursor.Decode(token, changed)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	var malformed sdk.Cursor
	err = json.Unmarshal([]byte(`{}`), &malformed)
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestUnboundPage(t *testing.T) {
	token, err := cursor.Encode(cursor.Binding{Product: "pixiv", Operation: "search"}, url.Values{"offset": {"30"}})
	require.NoError(t, err)
	page := sdk.NewPage([]int{1}, token, func(context.Context) (sdk.Page[int], error) { t.Fatal("retained binding"); return sdk.Page[int]{}, nil })
	data, err := json.Marshal(page)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &page))
	_, err = page.Next(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestErrorChainAndPrivacy(t *testing.T) {
	failure := &sdk.Error{Reason: sdk.LocalStateError, Operation: "save", Message: "credential save failed", Err: context.Canceled}
	require.ErrorIs(t, failure, context.Canceled)
	require.ErrorIs(t, failure, &sdk.Error{Reason: sdk.LocalStateError})
	require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(fmt.Errorf("wrapped: %w", failure)))
	failure.Err = errors.New("secret-fixture")
	for _, format := range []string{"%v", "%+v", "%#v", "%q"} {
		require.NotContains(t, fmt.Sprintf(format, failure), "secret-fixture")
	}
	require.Equal(t, "secret-fixture", errors.Unwrap(failure).Error())
	failure.Err = &url.Error{URL: "https://fixture.invalid?token=secret-fixture", Err: context.Canceled}
	data, err := json.Marshal(failure)
	require.NoError(t, err)
	require.NotContains(t, string(data), "secret-fixture")
	credentials := NewCredentials("credential-fixture")
	store, err := NewMemoryCredentials(credentials)
	require.NoError(t, err)
	client, err := New(store, Options{})
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	for _, format := range []string{"%v", "%+v", "%#v"} {
		require.NotContains(t, fmt.Sprintf(format, []any{credentials, store, client}), "credential-fixture")
	}
}
