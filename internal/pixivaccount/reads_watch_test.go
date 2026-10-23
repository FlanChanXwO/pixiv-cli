package pixivaccount_test

import (
	"context"
	"database/sql"
	pixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	poolpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount/pool"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
	sdkpixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type observedReadRepository struct {
	pixiv.Repository
	read func()
}

func (r observedReadRepository) GetPixiv(ctx context.Context, uid int64) (pixiv.Account, error) {
	r.read()
	return r.Repository.GetPixiv(ctx, uid)
}

func TestReadSessionsCloseCancelsWatcherWaitingForDatabaseLock(t *testing.T) {
	db, err := database.Open(t.TempDir())
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.SavePixivCredential(t.Context(), pixiv.New(42, "artist", []byte("refresh"))))
	var observe atomic.Bool
	var once sync.Once
	reading := make(chan struct{})
	repo := observedReadRepository{Repository: db, read: func() {
		if observe.Load() {
			once.Do(func() { close(reading) })
		}
	}}
	sessions := pixiv.NewReadSessions(pixiv.NewService(repo, &pixivTestDefaults{}), poolpixiv.NewGate())
	defer sessions.Close()
	options := sdkpixiv.Options{HTTPClient: &http.Client{Transport: pixivRoundTripper(func(*http.Request) (*http.Response, error) {
		return pixivJSONResponse(`{"access_token":"access","refresh_token":"rotated","expires_in":3600,"user":{"id":"42","name":"artist"}}`), nil
	})}}
	first, err := sessions.Open(t.Context(), 42, "network", options)
	require.NoError(t, err)
	require.NoError(t, sessions.Watch(t.Context(), db.Path()))
	held, release := make(chan struct{}), make(chan struct{})
	holder := make(chan error, 1)
	go func() {
		holder <- filelock.WithPrivateLock(t.Context(), db.Path(), func() error { close(held); <-release; return nil })
	}()
	defer func() { close(release); require.NoError(t, <-holder) }()
	<-held
	observe.Store(true)
	// 创建journal目录事件；观察查询被侧车锁阻塞，不执行任何fixture SQL变更。
	journal := db.Path() + "-journal"
	require.NoError(t, os.WriteFile(journal, nil, 0600))
	defer os.Remove(journal)
	select {
	case <-reading:
	case <-time.After(time.Second):
		t.Fatal("watcher did not start database revalidation")
	}
	done := make(chan error, 1)
	go func() { done <- sessions.Close() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("closing sessions waited for an unrelated database lock")
	}
	select {
	case <-first.Invalidated:
	default:
		t.Fatal("close did not cancel its read generation")
	}
}

func TestReadSessionsWatchExternalDatabaseCredentials(t *testing.T) {
	for _, mode := range []string{"delete", "wal", "metadata", "lost"} {
		t.Run(mode, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "database")
			reader, err := database.Open(dir)
			require.NoError(t, err)
			defer reader.Close()
			if mode == "wal" {
				control, err := sql.Open("sqlite", reader.Path())
				require.NoError(t, err)
				defer control.Close()
				var journal string
				require.NoError(t, control.QueryRow("PRAGMA journal_mode=WAL").Scan(&journal))
				require.Equal(t, "wal", journal)
			}
			writer, err := database.Open(dir)
			require.NoError(t, err)
			defer writer.Close()
			require.NoError(t, writer.SavePixivCredential(t.Context(), pixiv.New(42, "artist", []byte("refresh"))))
			sessions := pixiv.NewReadSessions(pixiv.NewService(reader, &pixivTestDefaults{}), poolpixiv.NewGate())
			defer sessions.Close()
			require.NoError(t, sessions.Watch(t.Context(), reader.Path()))
			var refreshes atomic.Int64
			options := sdkpixiv.Options{HTTPClient: &http.Client{Transport: pixivRoundTripper(func(*http.Request) (*http.Response, error) {
				refreshes.Add(1)
				return pixivJSONResponse(`{"access_token":"access","refresh_token":"rotated","expires_in":3600,"user":{"id":"42","name":"artist"}}`), nil
			})}}
			first, err := sessions.Open(t.Context(), 42, "network", options)
			require.NoError(t, err)
			if mode == "metadata" {
				require.NoError(t, writer.UpdatePixivMetadata(t.Context(), 42, "new name", nil, nil))
				select {
				case <-first.Invalidated:
					t.Fatal("metadata-only commit cancelled credentials")
				case <-time.After(100 * time.Millisecond):
				}
				require.NoError(t, sessions.Close())
				require.Equal(t, int64(1), refreshes.Load())
				return
			}
			if mode == "lost" {
				// Windows keeps SQLite files locked until both database handles close.
				if runtime.GOOS == "windows" {
					require.NoError(t, writer.Close())
					require.NoError(t, reader.Close())
				}
				require.NoError(t, os.Rename(dir, filepath.Join(filepath.Dir(dir), "moved")))
			} else if mode == "delete" {
				require.NoError(t, writer.RemovePixiv(t.Context(), 42))
			} else {
				require.NoError(t, writer.SavePixivCredential(t.Context(), pixiv.New(42, "artist", []byte("replacement"))))
			}
			select {
			case <-first.Invalidated:
			case <-time.After(3 * time.Second):
				t.Fatal("external database commit did not cancel the old read generation")
			}
			require.Equal(t, int64(1), refreshes.Load(), "watch must not refresh credentials")
			if mode == "lost" {
				require.EqualError(t, sessions.Close(), "read session credential watcher failed")
				_, err := sessions.Open(t.Context(), 42, "network", options)
				require.Error(t, err)
			} else {
				require.NoError(t, sessions.Close())
			}
		})
	}
}
