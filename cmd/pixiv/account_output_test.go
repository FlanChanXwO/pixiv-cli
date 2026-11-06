package main_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/stretchr/testify/require"
)

func TestAccountOutputsMatchLegacyFixtures(t *testing.T) {
	home := isolatedHome(t)
	store, err := account.Open(t.Context(), filepath.Join(home, config.AppDataDirName))
	require.NoError(t, err)
	premium := true
	token := credential.New([]byte("synthetic-refresh"))
	require.NoError(t, store.SavePixivCredentials(t.Context(), []account.Pixiv{
		{UserID: 42, Username: "fixture-artist", PremiumStatus: &premium, Credential: token},
		{UserID: 99, Username: "fixture-second", Credential: token},
		{UserID: 7, Username: "fixture-third", Credential: token},
	}))
	require.NoError(t, store.SetPixivSchedulable(t.Context(), []int64{99}, false))
	require.NoError(t, store.Freeze(t.Context(), 7, 4102444800))
	require.NoError(t, store.SaveFanboxCredential(t.Context(), account.Fanbox{UserID: 84, DisplayName: "fixture-fanbox", CreatorID: "fixture-creator", ValidatedAt: time.Unix(1, 0).UTC(), Credential: credential.New([]byte("synthetic-session"))}))
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"auth-list-text", []string{"auth", "list"}}, {"auth-list-json", []string{"auth", "list", "--json"}},
		{"whoami-text", []string{"whoami"}}, {"whoami-json", []string{"whoami", "--json"}},
		{"fanbox-status-text", []string{"fanbox", "auth", "status"}}, {"fanbox-status-json", []string{"fanbox", "auth", "status", "--json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, tc.args...)
			require.Zero(t, code, stderr)
			require.Empty(t, stderr)
			expected, err := os.ReadFile(filepath.Join("testdata", "accounts", tc.name+".golden"))
			require.NoError(t, err)
			require.Equal(t, string(expected), stdout)
		})
	}
}
