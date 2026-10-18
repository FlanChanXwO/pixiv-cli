package accounts_test

import (
	"context"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestInitialSelectionUsesOnlyExplicitDefaultOrSoleAccount(t *testing.T) {
	for _, test := range []struct {
		name  string
		local accounts.LocalSnapshot
		want  int64
		state string
	}{
		{"empty", accounts.LocalSnapshot{}, 0, "no_local_account"},
		{"sole", accounts.LocalSnapshot{Accounts: []accounts.Account{{UserID: 42, HasCredentials: true}}}, 42, "selected"},
		{"multiple", accounts.LocalSnapshot{Accounts: []accounts.Account{{UserID: 42, HasCredentials: true}, {UserID: 73, HasCredentials: true}}}, 0, "selection_required"},
		{"explicit", accounts.LocalSnapshot{DefaultUserID: 73, Accounts: []accounts.Account{{UserID: 42, HasCredentials: true}, {UserID: 73, HasCredentials: true}}}, 73, "selected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
			_, err := store.Init(t.Context(), false)
			require.NoError(t, err)
			manager := accounts.Manager{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) { return test.local, nil }}
			status, err := manager.Status(t.Context())
			require.NoError(t, err)
			require.Equal(t, test.want, status.SelectedUserID)
			require.Equal(t, test.state, status.SelectionState)
			saved, err := store.Read(t.Context())
			require.NoError(t, err)
			require.Equal(t, test.want, saved.SelectedPixivUserID)
		})
	}
}

func TestUsePersistsAcrossManagersAndDoesNotFallbackAfterRemoval(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	local := accounts.LocalSnapshot{DefaultUserID: 42, Accounts: []accounts.Account{{UserID: 42, HasCredentials: true}, {UserID: 73, HasCredentials: true}}}
	load := func(context.Context) (accounts.LocalSnapshot, error) { return local, nil }
	manager := accounts.Manager{Store: store, Load: load}
	status, err := manager.Use(t.Context(), 73)
	require.NoError(t, err)
	require.Equal(t, int64(73), status.SelectedUserID)
	other := accounts.Manager{Store: auth.Store{Path: store.Path}, Load: load}
	id, err := other.Resolve(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(73), id)
	local.Accounts = local.Accounts[:1]
	status, err = other.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(73), status.SelectedUserID)
	require.Equal(t, "account_not_found", status.SelectionState)
	_, err = other.Resolve(t.Context())
	require.ErrorContains(t, err, "account_not_found")
	_, err = other.Use(t.Context(), 999)
	require.Error(t, err)
	saved, err := store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(73), saved.SelectedPixivUserID)
	require.Equal(t, int64(42), local.DefaultUserID)
}

func TestMissingCredentialsDoNotImplyUpstreamValidityOrFallback(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	manager := accounts.Manager{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) {
		return accounts.LocalSnapshot{Accounts: []accounts.Account{{UserID: 42}}}, nil
	}}
	status, err := manager.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, "credentials_missing", status.SelectionState)
	require.Equal(t, "missing", status.CredentialState)
	_, err = manager.Resolve(t.Context())
	require.ErrorContains(t, err, "credentials_missing")
	_, err = manager.Use(t.Context(), 0)
	require.Error(t, err)
}

func TestLoginSelectionUsesLocalAvailability(t *testing.T) {
	for _, tc := range []struct {
		name           string
		selected       int64
		hasCredentials bool
		want           int64
		changed        bool
	}{
		{"unset", 0, true, 73, true}, {"available", 42, true, 42, false},
		{"missing", 99, true, 73, true}, {"missing-credentials", 42, false, 73, true},
		{"already-selected", 73, true, 73, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
			_, err := store.Init(t.Context(), false)
			require.NoError(t, err)
			if tc.selected != 0 {
				require.NoError(t, store.SelectPixivUser(t.Context(), tc.selected))
			}
			manager := accounts.Manager{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) {
				return accounts.LocalSnapshot{DefaultUserID: 42, Accounts: []accounts.Account{{UserID: 42, HasCredentials: tc.hasCredentials}, {UserID: 73, HasCredentials: true}}}, nil
			}}
			changed, err := manager.SelectAfterLogin(t.Context(), 73)
			require.NoError(t, err)
			require.Equal(t, tc.changed, changed)
			saved, err := store.Read(t.Context())
			require.NoError(t, err)
			require.Equal(t, tc.want, saved.SelectedPixivUserID)
		})
	}
}

func TestLoginSelectionSerializesWithConcurrentUse(t *testing.T) {
	for _, initial := range []int64{0, 42, 99} {
		store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
		_, err := store.Init(t.Context(), false)
		require.NoError(t, err)
		if initial != 0 {
			require.NoError(t, store.SelectPixivUser(t.Context(), initial))
		}
		manager := accounts.Manager{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) {
			return accounts.LocalSnapshot{Accounts: []accounts.Account{{UserID: 42, HasCredentials: true}, {UserID: 73, HasCredentials: true}}}, nil
		}}
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() { <-start; _, err := manager.SelectAfterLogin(t.Context(), 73); done <- err }()
		go func() { <-start; _, err := manager.Use(t.Context(), 42); done <- err }()
		close(start)
		require.NoError(t, <-done)
		require.NoError(t, <-done)
		state, err := store.Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, int64(42), state.SelectedPixivUserID)
	}
}

func TestLoginSelectionFailureDoesNotChangeState(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	require.NoError(t, store.SelectPixivUser(t.Context(), 42))
	original, err := store.Read(t.Context())
	require.NoError(t, err)
	manager := accounts.Manager{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) {
		return accounts.LocalSnapshot{}, context.Canceled
	}}
	changed, err := manager.SelectAfterLogin(t.Context(), 73)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, changed)
	after, err := store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, original, after)
}
