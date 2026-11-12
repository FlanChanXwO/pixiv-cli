package account_test

import (
	"bytes"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/stretchr/testify/require"
)

func TestMigrateV121PreservesAccountsAndLegacyFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "pixiv-cli.db")
	body, err := os.ReadFile(filepath.Join("testdata", "v1.2.1.db"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, body, 0600))
	before, err := os.Stat(path)
	require.NoError(t, err)
	store, err := account.Open(t.Context(), directory)
	require.NoError(t, err)
	pixiv, err := store.ListPixiv(t.Context())
	require.NoError(t, err)
	require.Len(t, pixiv, 1)
	require.Equal(t, int64(42), pixiv[0].UserID)
	require.Equal(t, "fixture-artist", pixiv[0].Username)
	require.Equal(t, int64(1), pixiv[0].CredentialRevision)
	require.True(t, pixiv[0].Pool.Enabled)
	require.Nil(t, pixiv[0].PremiumStatus)
	require.Nil(t, pixiv[0].PremiumCheckedAt)
	require.Nil(t, pixiv[0].Pool.FrozenUntil)
	require.Equal(t, time.Unix(1, 0).UTC(), pixiv[0].CreatedAt)
	require.Equal(t, pixiv[0].CreatedAt, pixiv[0].UpdatedAt)
	require.Equal(t, "fixture-refresh-token", pixiv[0].Credential.Value())
	fanbox, err := store.ListFanbox(t.Context())
	require.NoError(t, err)
	require.Empty(t, fanbox)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(body, after))
	stat, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), stat.ModTime())
}

func TestFileIdentityProtectsOwnedData(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		reject     bool
	}{
		{"missing", "", false}, {"foreign", `{"accounts":[]}`, false},
		{"invalid-json", `{"schema":`, true},
		{"damaged", `{"schema":"pixiv-cli.auth","version":1,"pixiv":[{"user_id":0}],"fanbox":[]}`, true},
		{"future", `{"schema":"pixiv-cli.auth","version":2,"pixiv":[],"fanbox":[]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "auth.json")
			if tc.body != "" {
				require.NoError(t, os.WriteFile(path, []byte(tc.body), 0600))
			}
			store, err := account.Open(t.Context(), directory)
			if tc.reject {
				require.Error(t, err)
				body, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, tc.body, string(body))
				return
			}
			require.NoError(t, err)
			pixiv, err := store.ListPixiv(t.Context())
			require.NoError(t, err)
			require.Empty(t, pixiv)
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			require.JSONEq(t, `{"schema":"pixiv-cli.auth","version":1,"pixiv_pool_last_user_id":0,"pixiv":[],"fanbox":[]}`, string(body))
			if runtime.GOOS != "windows" {
				stat, err := os.Stat(path)
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0600), stat.Mode().Perm())
			}
		})
	}
}

func TestLegacyIdentityAndVersionAreRequired(t *testing.T) {
	for _, statement := range []string{
		"PRAGMA application_id = 0", "PRAGMA application_id = 1234", "PRAGMA user_version = 0", "PRAGMA user_version = 4", "DELETE FROM schema_migration WHERE version = 2",
	} {
		t.Run(statement, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "pixiv-cli.db")
			body, err := os.ReadFile(filepath.Join("testdata", "v1.2.1.db"))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, body, 0600))
			escaped := (&url.URL{Path: filepath.ToSlash(path)}).EscapedPath()
			db, err := sql.Open("sqlite3", "file:"+escaped+"?mode=rw")
			require.NoError(t, err)
			_, err = db.Exec(statement)
			require.NoError(t, err)
			require.NoError(t, db.Close())
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = account.Open(t.Context(), directory)
			require.Error(t, err)
			require.NoFileExists(t, filepath.Join(directory, "auth.json"))
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.True(t, bytes.Equal(before, after))
		})
	}
}

func TestMigratedFileDoesNotReadLegacyAgain(t *testing.T) {
	directory := t.TempDir()
	_, err := account.Open(t.Context(), directory)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "pixiv-cli.db"), []byte("not a database"), 0600))
	store, err := account.Open(t.Context(), directory)
	require.NoError(t, err)
	pixiv, err := store.ListPixiv(t.Context())
	require.NoError(t, err)
	require.Empty(t, pixiv)
}

func TestMalformedPoolFilePreserved(t *testing.T) {
	for _, tc := range []struct{ name, pool string }{
		{"missing", ""},
		{"misspelled-freeze", `,"pool":{"enabled":true,"frozenUntil":"2026-11-06T00:00:00Z"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "auth.json")
			body := `{"schema":"pixiv-cli.auth","version":1,"pixiv":[{"user_id":1,"revision":1,"refresh_token":"test","created_at":"2026-11-06T00:00:00Z","updated_at":"2026-11-06T00:00:00Z"` + tc.pool + `}],"fanbox":[]}`
			require.NoError(t, os.WriteFile(path, []byte(body), 0600))
			_, err := account.Open(t.Context(), directory)
			require.Error(t, err)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, body, string(after))
		})
	}
}

func TestExistingAccountFileKeepsPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are enforced on Unix platforms")
	}
	directory := t.TempDir()
	_, err := account.Open(t.Context(), directory)
	require.NoError(t, err)
	path := filepath.Join(directory, "auth.json")
	require.NoError(t, os.Chmod(path, 0644))
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	before, err := os.Stat(path)
	require.NoError(t, err)
	_, err = account.Open(t.Context(), directory)
	require.NoError(t, err)
	stat, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), stat.Mode().Perm())
	require.Equal(t, before.ModTime(), stat.ModTime())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, body, after)
}

func TestEmptyCredentialsPreserveFile(t *testing.T) {
	for _, product := range []string{"pixiv", "fanbox"} {
		t.Run(product, func(t *testing.T) {
			directory := t.TempDir()
			store, err := account.Open(t.Context(), directory)
			require.NoError(t, err)
			require.NoError(t, store.SavePixivCredential(t.Context(), account.Pixiv{UserID: 42, Credential: credential.New([]byte("synthetic"))}))
			require.NoError(t, store.SaveFanboxCredential(t.Context(), account.Fanbox{UserID: 84, Credential: credential.New([]byte("synthetic")), ValidatedAt: time.Now().UTC()}))
			path := filepath.Join(directory, "auth.json")
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			field := "refresh_token"
			if product == "fanbox" {
				field = "session_id"
			}
			body = bytes.Replace(body, []byte("\""+field+"\": \"synthetic\""), []byte("\""+field+"\": \"\""), 1)
			require.NoError(t, os.WriteFile(path, body, 0600))
			_, err = account.Open(t.Context(), directory)
			require.Error(t, err)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, body, after)
		})
	}
}
