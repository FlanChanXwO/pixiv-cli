package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/stretchr/testify/require"
)

func TestAuthDefaultUserIDRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	restore := paths.SetConfigFilePathForTest(configPath)
	defer restore()
	t.Cleanup(func() { _ = os.Remove(configPath) })

	if _, ok, err := config.DefaultStore().ReadPixivDefaultUserID(); err != nil || ok {
		t.Fatalf("initial pixiv default: ok=%v err=%v", ok, err)
	}
	if err := config.DefaultStore().SetPixivDefaultUserID(123); err != nil {
		t.Fatalf("set pixiv: %v", err)
	}
	if userID, ok, err := config.DefaultStore().ReadPixivDefaultUserID(); err != nil || !ok || userID != 123 {
		t.Fatalf("read pixiv = %d, %v, %v", userID, ok, err)
	}
	if err := config.DefaultStore().ClearPixivDefaultUserID(); err != nil {
		t.Fatalf("clear pixiv: %v", err)
	}
	if _, ok, err := config.DefaultStore().ReadPixivDefaultUserID(); err != nil || ok {
		t.Fatalf("after clear: ok=%v err=%v", ok, err)
	}

	if err := config.DefaultStore().SetFanboxDefaultUserID(456); err != nil {
		t.Fatalf("set fanbox: %v", err)
	}
	if userID, ok, err := config.DefaultStore().ReadFanboxDefaultUserID(); err != nil || !ok || userID != 456 {
		t.Fatalf("read fanbox = %d, %v, %v", userID, ok, err)
	}
	if err := config.DefaultStore().ClearFanboxDefaultUserID(); err != nil {
		t.Fatalf("clear fanbox: %v", err)
	}
}

// TestAuthDefaultUserIDRejectsInvalid covers the write-side range check.
func TestAuthDefaultUserIDRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	restore := paths.SetConfigFilePathForTest(filepath.Join(dir, "config.toml"))
	defer restore()
	for _, userID := range []int64{0, -5} {
		if err := config.DefaultStore().SetPixivDefaultUserID(userID); err == nil {
			t.Fatalf("user id %d should be rejected", userID)
		}
	}
}

// TestInvalidDefaultUserIDDoesNotBreakOrdinaryRuntimeReads keeps the account
// selection out of Runtime(): only the on-demand read reports the bad value.
func TestInvalidDefaultUserIDDoesNotBreakOrdinaryRuntimeReads(t *testing.T) {
	withoutProxyEnvironment(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`[pixiv.auth]
default_user_id = -5

[fanbox.auth]
default_user_id = "not-a-number"

[download]
path = "./ok"
`), 0o600))

	state, err := currentSnapshot(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err, "an invalid default_user_id must not break ordinary runtime reads")
	require.Equal(t, "./ok", runtime.DownloadPath)

	for _, alias := range config.ValidSettingAliases() {
		_, err := state.Effective(alias)
		require.NoErrorf(t, err, "Effective(%q) must not be affected by default_user_id", alias)
	}

	store := config.Store{Files: diskFileStore{path: path}}
	_, _, err = store.ReadPixivDefaultUserID()
	require.Error(t, err, "reading the default user id must still surface the invalid value")
	require.Contains(t, err.Error(), "must be a positive integer")

	_, _, err = store.ReadFanboxDefaultUserID()
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be a positive integer")
}

// TestDefaultUserIDStaysOutOfRuntimeAndConfigAliases keeps the auth table
// readable and writable without exposing it as a regular setting.
func TestDefaultUserIDStaysOutOfRuntimeAndConfigAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	store := config.Store{Files: diskFileStore{path: path}}

	_, ok, err := store.ReadPixivDefaultUserID()
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, store.SetPixivDefaultUserID(12345))
	userID, ok, err := store.ReadPixivDefaultUserID()
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(12345), userID)

	state, err := currentSnapshot(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.NoError(t, err)
	for _, alias := range config.ValidSettingAliases() {
		require.NotEqual(t, "default_user_id", alias, "default_user_id must not become a config alias")
	}

	require.NoError(t, store.ClearPixivDefaultUserID())
	_, ok, err = store.ReadPixivDefaultUserID()
	require.NoError(t, err)
	require.False(t, ok)
}
