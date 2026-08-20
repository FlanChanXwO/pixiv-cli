package pool_test

import (
	"context"
	"errors"
	"testing"

	poolpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/pool"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/stretchr/testify/require"
)

type credentialStore struct {
	err          error
	called       bool
	userID       int64
	expected     int64
	refreshToken []byte
}

func (s *credentialStore) RotatePixivCredentials(_ context.Context, userID, expectedRevision int64, refreshToken []byte) error {
	s.called = true
	s.userID = userID
	s.expected = expectedRevision
	s.refreshToken = append([]byte(nil), refreshToken...)
	return s.err
}

func TestRotateCredentialChecksIdentityBeforeStore(t *testing.T) {
	store := &credentialStore{}
	err := poolpixiv.RotateCredential(context.Background(), store, 42, 43, 2, []byte("new-token"))
	require.Error(t, err)
	require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
	require.False(t, store.called)
	require.NotContains(t, err.Error(), "42")
}

func TestRotateCredentialUsesRevisionCAS(t *testing.T) {
	store := &credentialStore{err: errors.New("revision conflict")}
	err := poolpixiv.RotateCredential(context.Background(), store, 42, 42, 7, []byte("new-token"))
	require.ErrorIs(t, err, store.err)
	require.True(t, store.called)
	require.Equal(t, int64(42), store.userID)
	require.Equal(t, int64(7), store.expected)
	require.Equal(t, []byte("new-token"), store.refreshToken)
}

func TestRotateCredentialRejectsMissingStore(t *testing.T) {
	err := poolpixiv.RotateCredential(context.Background(), nil, 42, 42, 1, []byte("token"))
	require.EqualError(t, err, "pixiv credential repository is not configured")
}

func TestVerifyAccountIdentity(t *testing.T) {
	tests := []struct {
		name          string
		selected      int64
		authenticated int64
		wantError     bool
	}{
		{name: "matching identities", selected: 42, authenticated: 42},
		{name: "different identities", selected: 42, authenticated: 43, wantError: true},
		{name: "missing selected identity", authenticated: 42, wantError: true},
		{name: "missing authenticated identity", selected: 42, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := poolpixiv.VerifyAccountIdentity(test.selected, test.authenticated)
			if test.wantError {
				require.Error(t, err)
				require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
				return
			}
			require.NoError(t, err)
		})
	}
}
