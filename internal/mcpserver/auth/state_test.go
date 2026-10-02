package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/stretchr/testify/require"
)

func TestStoreReopensCurrentOwnerWithoutRawSecret(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "private", "mcp-state.json")}
	_, err := store.Read(t.Context())
	require.ErrorIs(t, err, auth.ErrNotInitialized)
	secret, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	entropy, err := base64.RawURLEncoding.DecodeString(secret)
	require.NoError(t, err)
	require.Len(t, entropy, 32)
	reopened := auth.Store{Path: store.Path}
	state, err := reopened.Read(t.Context())
	require.NoError(t, err)
	digest := sha256.Sum256([]byte(secret))
	require.True(t, state.OwnerVerifier == hex.EncodeToString(digest[:]), "wrong verifier")
	if runtime.GOOS != "windows" {
		for filename, mode := range map[string]os.FileMode{store.Path: 0600, store.Path + ".lock": 0600, filepath.Dir(store.Path): 0700} {
			info, err := os.Stat(filename)
			require.NoError(t, err)
			require.Equal(t, mode, info.Mode().Perm())
		}
	}
	next, err := reopened.Init(t.Context(), true)
	require.NoError(t, err)
	require.False(t, secret == next, "reset reused secret")
	state, err = store.Read(t.Context())
	require.NoError(t, err)
	require.False(t, state.OwnerVerifier == hex.EncodeToString(digest[:]), "old store cached owner")
	body, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	require.False(t, bytes.Contains(body, []byte(secret)) || bytes.Contains(body, []byte(next)))
}

func TestStoreRejectsCorruptStateWithoutResettingIt(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(map[string]any)
		raw    string
	}{
		{name: "empty", raw: " "},
		{name: "null", raw: "null"},
		{name: "malformed", raw: "{fixture-secret"},
		{name: "unsupported version", change: func(s map[string]any) { s["version"] = 2 }},
		{name: "missing verifier", change: func(s map[string]any) { delete(s, "owner_verifier") }},
		{name: "raw verifier", change: func(s map[string]any) { s["owner_verifier"] = "fixture-secret" }},
		{name: "missing selection", change: func(s map[string]any) { delete(s, "selected_pixiv_user_id") }},
		{name: "null selection", change: func(s map[string]any) { s["selected_pixiv_user_id"] = nil }},
		{name: "incomplete client", change: func(s map[string]any) { s["clients"] = map[string]any{"client": map[string]any{}} }},
		{name: "incomplete grant", change: func(s map[string]any) { s["grants"] = map[string]any{"grant": map[string]any{}} }},
		{name: "missing clients", change: func(s map[string]any) { delete(s, "clients") }},
		{name: "null grants", change: func(s map[string]any) { s["grants"] = nil }},
		{name: "invalid selection", change: func(s map[string]any) { s["selected_pixiv_user_id"] = -1 }},
		{name: "unknown secret field", change: func(s map[string]any) { s["raw_token"] = "fixture-secret" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := auth.Store{Path: filepath.Join(t.TempDir(), "mcp-state.json")}
			_, err := store.Init(t.Context(), false)
			require.NoError(t, err)
			body, err := os.ReadFile(store.Path)
			require.NoError(t, err)
			if test.change != nil {
				var state map[string]any
				require.NoError(t, json.Unmarshal(body, &state))
				test.change(state)
				body, err = json.Marshal(state)
				require.NoError(t, err)
			} else {
				body = []byte(test.raw)
			}
			require.NoError(t, os.WriteFile(store.Path, body, 0600))
			_, err = store.Read(t.Context())
			require.Error(t, err, "corrupt state accepted")
			require.NotContains(t, err.Error(), "fixture-secret")
			for _, reset := range []bool{false, true} {
				secret, err := store.Init(t.Context(), reset)
				require.Error(t, err)
				require.Empty(t, secret)
				require.NotContains(t, err.Error(), "fixture-secret")
				unchanged, err := os.ReadFile(store.Path)
				require.NoError(t, err)
				require.True(t, bytes.Equal(body, unchanged), "corrupt state was overwritten")
			}
		})
	}
}

func TestStoreRejectsUnsafeFileAndCanceledMutation(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "mcp-state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	body, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	secret, err := store.Init(ctx, true)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, secret)
	unchanged, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(body, unchanged))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Chmod(store.Path, 0644))
		_, err = store.Read(t.Context())
		require.Error(t, err, "public state accepted")
		secret, err = store.Init(t.Context(), true)
		require.Error(t, err)
		require.Empty(t, secret)
		require.NoError(t, os.Chmod(store.Path, 0600))
		link := auth.Store{Path: store.Path + "-link"}
		require.NoError(t, os.Symlink(store.Path, link.Path))
		_, err = link.Read(t.Context())
		require.Error(t, err, "symlink accepted")
		secret, err = link.Init(t.Context(), true)
		require.Error(t, err)
		require.Empty(t, secret)
	}
}

func TestStoreConcurrentInitializationAndReset(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "mcp-state.json")
	type outcome struct {
		secret string
		err    error
	}
	for _, reset := range []bool{false, true} {
		results := make(chan outcome, 8)
		start := make(chan struct{})
		var workers sync.WaitGroup
		for range 8 {
			workers.Go(func() {
				<-start
				secret, err := (auth.Store{Path: filename}).Init(t.Context(), reset)
				results <- outcome{secret, err}
			})
		}
		close(start)
		workers.Wait()
		close(results)
		verifiers := map[string]bool{}
		for result := range results {
			if result.err != nil {
				require.False(t, reset, "reset failed")
				require.True(t, errors.Is(result.err, auth.ErrAlreadyInitialized))
				require.Empty(t, result.secret)
			} else {
				digest := sha256.Sum256([]byte(result.secret))
				hash := hex.EncodeToString(digest[:])
				require.False(t, verifiers[hash], "duplicate owner secret")
				verifiers[hash] = true
			}
		}
		expected := 1
		if reset {
			expected = 8
		}
		require.Len(t, verifiers, expected)
		state, err := (auth.Store{Path: filename}).Read(t.Context())
		require.NoError(t, err)
		require.True(t, verifiers[state.OwnerVerifier], "persisted owner was not a successful commit")
	}
}

func TestStoreEmptyPathHasNoFilesystemSideEffect(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	secret, err := (auth.Store{}).Init(t.Context(), false)
	require.Error(t, err)
	require.Empty(t, secret)
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, entries, "invalid store created files in working directory")
}

func TestStoreRequiresExplicitGrantRevocationAndRefreshHistory(t *testing.T) {
	for _, test := range []struct {
		name, field, value string
	}{
		{"missing revocation", "revoked", ""},
		{"null revocation", "revoked", "null"},
		{"string revocation", "revoked", `"fixture-secret"`},
		{"numeric revocation", "revoked", "0"},
		{"missing history", "used_refresh_hashes", ""},
		{"string history", "used_refresh_hashes", `"fixture-secret"`},
		{"object history", "used_refresh_hashes", `{}`},
		{"null history entry", "used_refresh_hashes", `[null]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
			_, err := store.Init(t.Context(), false)
			require.NoError(t, err)
			state, err := store.Read(t.Context())
			require.NoError(t, err)
			state.Clients["fixture"] = auth.Client{RedirectURIs: []string{"https://fixture.test/callback"}}
			state.Grants["fixture"] = auth.Grant{ClientID: "fixture", Resource: "https://instance.test/mcp", Scope: "mcp", RefreshHash: strings.Repeat("a", 64), AccessTokens: map[string]time.Time{}, UsedRefreshHashes: []string{strings.Repeat("b", 64)}, Revoked: true}
			body, err := json.Marshal(state)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(store.Path, body, 0600))
			_, err = store.Read(t.Context())
			require.NoError(t, err, "complete grant must be valid before corruption")
			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &raw))
			var grants map[string]map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw["grants"], &grants))
			if test.value == "" {
				delete(grants["fixture"], test.field)
			} else {
				grants["fixture"][test.field] = json.RawMessage(test.value)
			}
			raw["grants"], err = json.Marshal(grants)
			require.NoError(t, err)
			body, err = json.Marshal(raw)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(store.Path, body, 0600))
			reopened := auth.Store{Path: store.Path}
			_, err = reopened.Read(t.Context())
			require.ErrorIs(t, err, auth.ErrInvalidState)
			for _, reset := range []bool{false, true} {
				secret, err := reopened.Init(t.Context(), reset)
				require.ErrorIs(t, err, auth.ErrInvalidState)
				require.Empty(t, secret)
			}
			handler, err := auth.NewHandler("https://instance.test", reopened)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, "https://instance.test/oauth/register", strings.NewReader(`{"redirect_uris":["https://new.test/callback"]}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusInternalServerError, recorder.Code)
			require.JSONEq(t, `{"error":"server_error"}`, recorder.Body.String())
			unchanged, err := os.ReadFile(store.Path)
			require.NoError(t, err)
			require.Equal(t, body, unchanged, "invalid grant was overwritten")
		})
	}
}

func TestStorePreservesExplicitGrantRevocationAndEmptyHistory(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		for _, history := range [][]string{nil, {}, {strings.Repeat("b", 64)}} {
			store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
			_, err := store.Init(t.Context(), false)
			require.NoError(t, err)
			state, err := store.Read(t.Context())
			require.NoError(t, err)
			state.Clients["fixture"] = auth.Client{RedirectURIs: []string{"https://fixture.test/callback"}}
			grant := auth.Grant{ClientID: "fixture", Resource: "https://instance.test/mcp", Scope: "mcp", RefreshHash: strings.Repeat("a", 64), AccessTokens: map[string]time.Time{}, UsedRefreshHashes: history, Revoked: revoked}
			state.Grants["fixture"] = grant
			body, err := json.Marshal(state)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(store.Path, body, 0600))
			reopened := auth.Store{Path: store.Path}
			got, err := reopened.Read(t.Context())
			require.NoError(t, err)
			require.Equal(t, grant, got.Grants["fixture"])
			handler, err := auth.NewHandler("https://instance.test", reopened)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, "https://instance.test/oauth/register", strings.NewReader(`{"redirect_uris":["https://new.test/callback"]}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusCreated, recorder.Code)
			got, err = reopened.Read(t.Context())
			require.NoError(t, err)
			require.Equal(t, grant, got.Grants["fixture"], "registration changed grant status/history")
		}
	}
}

func TestStoreSelectPixivUserPersistsWithoutChangingOAuth(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	before, err := store.Read(t.Context())
	require.NoError(t, err)
	before.Clients["fixture"] = auth.Client{RedirectURIs: []string{"https://fixture.test/callback"}}
	before.Grants["fixture"] = auth.Grant{ClientID: "fixture", Resource: "https://instance.test/mcp", Scope: "mcp", RefreshHash: strings.Repeat("a", 64), AccessTokens: map[string]time.Time{}, UsedRefreshHashes: []string{strings.Repeat("b", 64)}, Revoked: true}
	body, err := json.Marshal(before)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.Path, body, 0600))
	require.NoError(t, store.SelectPixivUser(t.Context(), 42))
	after, err := (auth.Store{Path: store.Path}).Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(42), after.SelectedPixivUserID)
	before.SelectedPixivUserID = 42
	require.Equal(t, before, after)
	require.NoError(t, store.SelectPixivUser(t.Context(), 73))
	after, err = store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(73), after.SelectedPixivUserID)
}

func TestStoreSelectPixivUserRejectsInvalidIDWithoutMutation(t *testing.T) {
	for _, id := range []int64{0, -1} {
		store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
		_, err := store.Init(t.Context(), false)
		require.NoError(t, err)
		before, err := os.ReadFile(store.Path)
		require.NoError(t, err)
		require.Error(t, store.SelectPixivUser(t.Context(), id))
		after, err := os.ReadFile(store.Path)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}

func TestStoreSelectPixivUserEmptyPathHasNoSideEffects(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	require.Error(t, (auth.Store{}).SelectPixivUser(t.Context(), 42))
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestStoreSelectPixivUserSerializesWithOAuthRegistrationAndReset(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	handler, err := auth.NewHandler("https://instance.test", store)
	require.NoError(t, err)
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			request := httptest.NewRequest(http.MethodPost, "https://instance.test/oauth/register", strings.NewReader(`{"redirect_uris":["https://fixture.test/callback"]}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusCreated {
				t.Errorf("registration status = %d", response.Code)
			}
		})
		workers.Go(func() {
			if err := (auth.Store{Path: store.Path}).SelectPixivUser(t.Context(), 42); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	got, err := store.Read(t.Context())
	require.NoError(t, err)
	require.Len(t, got.Clients, 8)
	require.Equal(t, int64(42), got.SelectedPixivUserID)
	_, err = store.Init(t.Context(), true)
	require.NoError(t, err)
	got, err = store.Read(t.Context())
	require.NoError(t, err)
	require.Len(t, got.Clients, 8)
	require.Equal(t, int64(42), got.SelectedPixivUserID)
}

func TestStoreSelectPixivUserDoesNotRepairOrIgnoreCanceledState(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	require.ErrorIs(t, store.SelectPixivUser(t.Context(), 42), auth.ErrNotInitialized)
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	before, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, store.SelectPixivUser(ctx, 42), context.Canceled)
	after, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, os.WriteFile(store.Path, []byte("broken"), 0600))
	require.ErrorIs(t, store.SelectPixivUser(t.Context(), 42), auth.ErrInvalidState)
	after, err = os.ReadFile(store.Path)
	require.NoError(t, err)
	require.Equal(t, []byte("broken"), after)
}
