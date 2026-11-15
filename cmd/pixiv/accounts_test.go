package main_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
	fanboxauth "github.com/FlanChanXwO/pixiv-cli/internal/cli/fanbox/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
	"github.com/stretchr/testify/require"
)

func TestConfiguredDefaultPreservesReadErrors(t *testing.T) {
	for _, failure := range []string{"cancelled", "invalid-json"} {
		t.Run(failure, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			store, err := account.Open(t.Context(), filepath.Join(home, config.AppDataDirName))
			require.NoError(t, err)
			defaults := config.Store{Files: secretfile.ConfigFiles{}}
			pixiv := auth.NewAccounts(store, defaults)
			fanbox := fanboxauth.NewAccounts(store, defaults, nil)
			require.NoError(t, defaults.SetPixivDefaultUserID(42))
			require.NoError(t, defaults.SetFanboxDefaultUserID(84))
			ctx := t.Context()
			file := filepath.Join(home, config.AppDataDirName, "auth.json")
			if failure == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if failure == "invalid-json" {
				require.NoError(t, os.WriteFile(file, []byte("{invalid"), 0600))
			}
			_, pixivErr := pixiv.CurrentUser(ctx)
			_, fanboxErr := fanbox.Status(ctx)
			for _, err := range []error{pixivErr, fanboxErr} {
				switch failure {
				case "cancelled":
					require.ErrorIs(t, err, context.Canceled)
				case "invalid-json":
					require.ErrorContains(t, err, "account: invalid JSON")
				}
			}
		})
	}
}

func TestFailedRemovalPreservesExplicitDefault(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			store, err := account.Open(t.Context(), filepath.Join(home, config.AppDataDirName))
			require.NoError(t, err)
			defaults := config.Store{Files: secretfile.ConfigFiles{}}
			pixiv := auth.NewAccounts(store, defaults)
			require.NoError(t, defaults.SetPixivDefaultUserID(42))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if cancelled {
				cancel()
			} else {
				require.NoError(t, os.WriteFile(filepath.Join(home, config.AppDataDirName, "auth.json"), []byte("{invalid"), 0600))
			}
			require.Error(t, pixiv.RemoveAccount(ctx, 42))
			id, explicit, err := defaults.ReadPixivDefaultUserID()
			require.NoError(t, err)
			require.True(t, explicit)
			require.EqualValues(t, 42, id)
		})
	}
}
