package pixiv_test

import (
	"context"
	"net/http"
	"testing"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	poolpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/pool"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	sdkpixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

type changedAfterRotationRepository struct {
	accountpixiv.Repository
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
			require.NoError(t, db.SavePixivCredential(t.Context(), accountpixiv.New(42, "artist", []byte("refresh"))))
			var sessions *accountpixiv.ReadSessions
			repo := changedAfterRotationRepository{Repository: db, after: func() error {
				if change == "replace" {
					if err := db.SavePixivCredential(t.Context(), accountpixiv.New(42, "artist", []byte("replacement"))); err != nil {
						return err
					}
				} else if err := db.RemovePixiv(t.Context(), 42); err != nil {
					return err
				}
				// 变更通知先于新会话发布，不能依赖稍后再来一个文件事件。
				return sessions.Revalidate(t.Context())
			}}
			sessions = accountpixiv.NewReadSessions(accountpixiv.NewService(repo, &pixivTestDefaults{}), poolpixiv.NewGate())
			defer sessions.Close()
			refreshes := 0
			options := sdkpixiv.Options{HTTPClient: &http.Client{Transport: pixivRoundTripper(func(*http.Request) (*http.Response, error) {
				refreshes++
				return pixivJSONResponse(`{"access_token":"access","refresh_token":"rotated","expires_in":3600,"user":{"id":"42","name":"artist"}}`), nil
			})}}
			session, err := sessions.Open(t.Context(), 42, "network", options)
			if change == "replace" {
				require.ErrorIs(t, err, accountpixiv.ErrCredentialConflict)
			} else {
				require.ErrorIs(t, err, accountpixiv.ErrNotFound)
			}
			require.Nil(t, session.Client, "obsolete client must not be published")
			require.Equal(t, 1, refreshes, "publication failure must not retry refresh")
		})
	}
}
