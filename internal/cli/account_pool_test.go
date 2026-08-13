package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	filesecret "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret"
	"github.com/stretchr/testify/require"
)

type poolCapableFakeAccountStore struct {
	*fakeAccountStore
}

func valueOrFalse(value *bool) bool { return value != nil && *value }

func (s *poolCapableFakeAccountStore) SetPoolSchedulable(_ context.Context, userIDs []int64, schedulable bool) error {
	indices := make(map[int64]int, len(userIDs))
	for index, account := range s.accounts {
		indices[account.UserID] = index
	}
	for _, userID := range userIDs {
		if _, ok := indices[userID]; !ok {
			return errors.New("account not found")
		}
	}
	for _, userID := range userIDs {
		account := s.accounts[indices[userID]]
		account.Schedulable = schedulable
		account.Eligible = schedulable
		account.PoolStatusKnown = true
		s.accounts[indices[userID]] = account
	}
	return nil
}

func (s *poolCapableFakeAccountStore) SetAllPoolSchedulable(_ context.Context, schedulable bool) error {
	for index := range s.accounts {
		s.accounts[index].Schedulable = schedulable
		s.accounts[index].Eligible = schedulable
		s.accounts[index].PoolStatusKnown = true
	}
	return nil
}

func TestAuthPoolStatusEnableDisableAndAtomicUIDValidation(t *testing.T) {
	_, configPath := useTempPaths(t)
	require.NoError(t, filesecret.WritePrivateFile(configPath, []byte("[account_pool]\nenabled=true\nstrategy='random'\n"), localstate.PrivateFileMode))
	store := &poolCapableFakeAccountStore{fakeAccountStore: &fakeAccountStore{accounts: []pixivapp.AccountSummary{
		{UserID: 11, Username: "first", Schedulable: true, Eligible: true, PoolStatusKnown: true},
		{UserID: 22, Username: "second", Schedulable: true, Eligible: true, PoolStatusKnown: true},
	}}}
	setTestAccountStore(t, store)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "pool", "status", "--json"}, nil, &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	var status accountPoolStatusOut
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &status))
	require.True(t, status.Enabled)
	require.Equal(t, "random", status.Strategy)
	require.Len(t, status.Accounts, 2)
	require.True(t, valueOrFalse(status.Accounts[0].Schedulable))
	require.True(t, valueOrFalse(status.Accounts[0].Eligible))
	require.NotContains(t, stdout.String(), "refresh_token")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "list", "--json"}, nil, &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	require.Contains(t, stdout.String(), "schedulable")
	require.Contains(t, stdout.String(), "eligible")
	require.NotContains(t, stdout.String(), "refresh_token")

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "pool", "disable", "11", "22", "--json"}, nil, &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	require.Contains(t, stdout.String(), `"schedulable": false`)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "pool", "enable", "11", "999"}, nil, &stdout, &stderr)
	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "account not found")
	require.False(t, store.accounts[0].Schedulable, "unknown UID must not partially enable the batch")
	require.False(t, store.accounts[1].Schedulable)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "pool", "enable", "--all"}, nil, &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	require.True(t, store.accounts[0].Schedulable)
	require.True(t, store.accounts[1].Schedulable)

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "auth", "pool", "disable", "--all", "11"}, nil, &stdout, &stderr)
	require.NotZero(t, code)
	require.Contains(t, stderr.String(), "--all cannot be combined")
}
