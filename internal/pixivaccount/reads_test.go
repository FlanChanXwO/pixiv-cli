package pixivaccount_test

import (
	"context"
	pixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	poolpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount/pool"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	sdkpixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

type changedAfterRotationRepository struct {
	pixiv.Repository
	after func() error
}

func (r changedAfterRotationRepository) RotatePixivCredentials(ctx context.Context, uid, revision int64, token []byte) error {
	if err := r.Repository.RotatePixivCredentials(ctx, uid, revision, token); err != nil {
		return err
	}
	return r.after()
}

func TestReadSessionRejectsCredentialChangeBeforePublication(t *testing.T) {
	for _, change := range []string{"replace", "delete"} {
		t.Run(change, func(t *testing.T) {
			db, err := database.Open(t.TempDir())
			require.NoError(t, err)
			defer db.Close()
			require.NoError(t, db.SavePixivCredential(t.Context(), pixiv.New(42, "artist", []byte("refresh"))))
			var sessions *pixiv.ReadSessions
			repo := changedAfterRotationRepository{Repository: db, after: func() error {
				if change == "replace" {
					if err := db.SavePixivCredential(t.Context(), pixiv.New(42, "artist", []byte("replacement"))); err != nil {
						return err
					}
				} else if err := db.RemovePixiv(t.Context(), 42); err != nil {
					return err
				}
				// 变更通知先于新会话发布，不能依赖稍后再来一个文件事件。
				return sessions.Revalidate(t.Context())
			}}
			sessions = pixiv.NewReadSessions(pixiv.NewService(repo, &pixivTestDefaults{}), poolpixiv.NewGate())
			defer sessions.Close()
			refreshes := 0
			options := sdkpixiv.Options{HTTPClient: &http.Client{Transport: pixivRoundTripper(func(*http.Request) (*http.Response, error) {
				refreshes++
				return pixivJSONResponse(`{"access_token":"access","refresh_token":"rotated","expires_in":3600,"user":{"id":"42","name":"artist"}}`), nil
			})}}
			session, err := sessions.Open(t.Context(), 42, "network", options)
			if change == "replace" {
				require.ErrorIs(t, err, pixiv.ErrCredentialConflict)
			} else {
				require.ErrorIs(t, err, pixiv.ErrNotFound)
			}
			require.Nil(t, session.Client, "obsolete client must not be published")
			require.Equal(t, 1, refreshes, "publication failure must not retry refresh")
		})
	}
}

func TestReadSessionCancelledCredentialCheckPreservesSharedSession(t *testing.T) {
	db, err := database.Open(t.TempDir())
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.SavePixivCredential(t.Context(), pixiv.New(42, "artist", []byte("refresh"))))
	var cancelCheck context.CancelFunc
	repo := observedReadRepository{Repository: db, read: func() {
		if cancelCheck != nil {
			cancelCheck()
		}
	}}
	sessions := pixiv.NewReadSessions(pixiv.NewService(repo, &pixivTestDefaults{}), poolpixiv.NewGate())
	defer sessions.Close()
	refreshes := 0
	options := sdkpixiv.Options{HTTPClient: &http.Client{Transport: pixivRoundTripper(func(*http.Request) (*http.Response, error) {
		refreshes++
		return pixivJSONResponse(`{"access_token":"access","refresh_token":"rotated","expires_in":3600,"user":{"id":"42","name":"artist"}}`), nil
	})}}
	first, err := sessions.Open(t.Context(), 42, "network", options)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cancelCheck = cancel
	_, err = sessions.Open(ctx, 42, "network", options)
	require.ErrorIs(t, err, context.Canceled)
	select {
	case <-first.Invalidated:
		t.Fatal("one cancelled credential check invalidated unrelated readers")
	default:
	}
	cancelCheck = nil
	next, err := sessions.Open(t.Context(), 42, "network", options)
	require.NoError(t, err)
	require.Equal(t, first.Version, next.Version)
	require.Same(t, first.Client, next.Client)
	require.Equal(t, 1, refreshes, "cancelled read must not force another OAuth rotation")
}
