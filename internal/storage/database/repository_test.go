package database_test

import (
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/stretchr/testify/require"
)

func TestCredentialRevisionRejectsStaleRotation(t *testing.T) {
	db, err := database.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SavePixivCredential(t.Context(), pixivaccount.New(42, "artist", []byte("old"))))
	account, err := db.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.NoError(t, db.RotatePixivCredentials(t.Context(), 42, account.CredentialRevision, []byte("rotated")))
	err = db.RotatePixivCredentials(t.Context(), 42, account.CredentialRevision, []byte("stale"))
	require.ErrorIs(t, err, pixivaccount.ErrCredentialConflict)
	saved, err := db.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.Equal(t, account.CredentialRevision+1, saved.CredentialRevision)
	require.Equal(t, "rotated", string(saved.RefreshTokenCopy()))
}

func TestFailedSelectionRollsBackExpiredFreeze(t *testing.T) {
	db, err := database.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SavePixivCredential(t.Context(), pixivaccount.New(42, "artist", []byte("token"))))
	require.NoError(t, db.Freeze(t.Context(), 42, 100))
	before, err := db.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	abort := errors.New("selection aborted")
	_, err = db.SelectPixiv(t.Context(), 100, nil, func(snapshot pixivaccount.PoolSnapshot) (int64, error) {
		require.Len(t, snapshot.Candidates, 1)
		require.Nil(t, snapshot.Candidates[0].PoolFrozenUntil)
		return 0, abort
	})
	require.ErrorIs(t, err, abort)
	after, err := db.GetPixiv(t.Context(), 42)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
