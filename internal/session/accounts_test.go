package session_test

import (
	"context"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
	"os"
	"path/filepath"
	"testing"
	"time"

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
			pixiv, _, err := local.PixivAccounts(t.Context())
			require.NoError(t, err)
			fanbox, err := local.FanboxAccounts(t.Context())
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

func TestFailedRemovalPreservesExplicitDefault(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			pixiv, _, err := session.NewLocal(nil, nil, nil, nil).PixivAccounts(t.Context())
			require.NoError(t, err)
			defaults := config.Store{Files: secretfile.ConfigFiles{}}
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

func TestInitializationCancellationCanRetry(t *testing.T) {
	for _, localAccounts := range []bool{false, true} {
		t.Run(fmt.Sprint(localAccounts), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			release, locked, err := filelock.TryPrivateLock(filepath.Join(home, config.AppDataDirName, "auth.json"))
			require.NoError(t, err)
			require.True(t, locked)
			closeState := &lifecycle.CloseState{}
			defer closeState.Close()
			local := session.NewLocal(closeState, nil, nil, nil)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if localAccounts {
					_, err := local.LocalAccounts(ctx)
					done <- err
				} else {
					_, err := local.CurrentUser(ctx)
					done <- err
				}
			}()
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				require.NoError(t, release())
				<-done
				t.Fatal("initialization ignored cancellation while waiting for the account lock")
			}
			require.NoError(t, release())
			_, err = local.LocalAccounts(t.Context())
			require.NoError(t, err)
		})
	}
}
