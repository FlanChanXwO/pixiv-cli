package session_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
	"github.com/stretchr/testify/require"
)

func TestConfiguredDefaultPreservesReadErrors(t *testing.T) {
	for _, failure := range []string{"cancelled", "invalid-json"} {
		t.Run(failure, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			local := session.NewLocal(nil, nil, nil, nil)
			pixiv, _, err := local.PixivAccounts()
			require.NoError(t, err)
			fanbox, err := local.FanboxAccounts()
			require.NoError(t, err)
			defaults := config.Store{Files: secretfile.ConfigFiles{}}
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
