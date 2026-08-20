package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

type stubAccountPort struct {
	restoreInputs []pixivaccount.RestoreAccountInput
	restoreResult pixivaccount.RestoreAccountsResult
	restoreErr    error
}

func (s *stubAccountPort) ImportAccountWith(context.Context, string, bool, pixiv.Options) (pixivaccount.AccountSummary, error) {
	return pixivaccount.AccountSummary{}, errors.New("not used")
}
func (s *stubAccountPort) ListAccounts(context.Context) ([]pixivaccount.AccountSummary, error) {
	return nil, nil
}
func (s *stubAccountPort) UseAccount(context.Context, int64) error { return nil }
func (s *stubAccountPort) RemoveAccount(context.Context, int64) error {
	return nil
}
func (s *stubAccountPort) CheckAccountWith(context.Context, int64, pixiv.Options) (pixivaccount.AccountSummary, error) {
	return pixivaccount.AccountSummary{}, nil
}
func (s *stubAccountPort) RefreshAccountWith(context.Context, int64, pixiv.Options) (pixivaccount.AccountSummary, error) {
	return pixivaccount.AccountSummary{}, nil
}
func (s *stubAccountPort) CurrentUser(context.Context) (*pixivaccount.AccountSummary, error) {
	return nil, nil
}
func (s *stubAccountPort) RestoreAccount(context.Context, pixivaccount.AccountSummary, string, bool) error {
	return nil
}
func (s *stubAccountPort) RestoreAccounts(_ context.Context, inputs []pixivaccount.RestoreAccountInput) (pixivaccount.RestoreAccountsResult, error) {
	s.restoreInputs = inputs
	if s.restoreErr != nil {
		return pixivaccount.RestoreAccountsResult{}, s.restoreErr
	}
	if s.restoreResult.Accounts == nil {
		out := make([]pixivaccount.RestoreAccountOutcome, 0, len(inputs))
		for _, input := range inputs {
			out = append(out, pixivaccount.RestoreAccountOutcome{Account: input.Account})
		}
		s.restoreResult.Accounts = out
	}
	return s.restoreResult, nil
}
func (s *stubAccountPort) AccountsWithTokens(context.Context) ([]pixivaccount.AccountWithToken, error) {
	return nil, nil
}
func (s *stubAccountPort) SetPoolSchedulable(context.Context, []int64, bool) error { return nil }
func (s *stubAccountPort) SetAllPoolSchedulable(context.Context, bool) error       { return nil }

func newStubService(port *stubAccountPort) AccountService {
	return AccountService{Pixiv: port}
}

func validBundleJSON() string {
	return `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":42,"accounts":[{"user_id":42,"username":"alice","refresh_token":"t42"},{"user_id":7,"username":"bob","refresh_token":"t7"}]}`
}

func TestImportBundleForwardsBundleDefaultFlag(t *testing.T) {
	port := &stubAccountPort{}
	service := newStubService(port)
	port.restoreResult = pixivaccount.RestoreAccountsResult{
		Accounts: []pixivaccount.RestoreAccountOutcome{
			{Account: pixivaccount.AccountSummary{UserID: 42, Username: "alice"}, IsReplacement: true},
			{Account: pixivaccount.AccountSummary{UserID: 7, Username: "bob"}, IsReplacement: false},
		},
		ResultingDefault: 42,
	}

	result, err := service.ImportBundle([]byte(validBundleJSON()))
	require.NoError(t, err)
	require.Equal(t, int64(42), result.DefaultUserID)
	require.Len(t, result.Accounts, 2)
	require.Equal(t, "updated", result.Accounts[0].Status)
	require.Equal(t, "added", result.Accounts[1].Status)
	require.True(t, port.restoreInputs[0].IsBundleDefault)
	require.False(t, port.restoreInputs[1].IsBundleDefault)
}

func TestDecodeAuthExportBundleRejectsUnknownFields(t *testing.T) {
	body := `{"schema":"pixiv-cli.auth-export","version":1,"accounts":[{"user_id":42,"username":"a","refresh_token":"t","extra":1}]}`
	_, err := decodeAuthExportBundle([]byte(body))
	require.Error(t, err)
}

func TestDecodeAuthExportBundleRejectsDuplicateAccountUIDs(t *testing.T) {
	body := `{"schema":"pixiv-cli.auth-export","version":1,"accounts":[{"user_id":42,"username":"a","refresh_token":"t1"},{"user_id":42,"username":"b","refresh_token":"t2"}]}`
	_, err := decodeAuthExportBundle([]byte(body))
	require.ErrorContains(t, err, "duplicate account")
}

func TestDecodeAuthExportBundleRejectsDuplicateObjectKeys(t *testing.T) {
	body := `{"schema":"pixiv-cli.auth-export","version":1,"accounts":[{"user_id":42,"user_id":43,"username":"a","refresh_token":"t"}]}`
	_, err := decodeAuthExportBundle([]byte(body))
	require.ErrorContains(t, err, "duplicate object key")
}

func TestDecodeAuthExportBundleRejectsTrailingJSON(t *testing.T) {
	body := validBundleJSON() + ` {"extra":1}`
	_, err := decodeAuthExportBundle([]byte(body))
	require.ErrorContains(t, err, "trailing")
}

func TestDecodeAuthExportBundleRejectsDefaultNotInAccounts(t *testing.T) {
	body := `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":99,"accounts":[{"user_id":42,"username":"a","refresh_token":"t"}]}`
	_, err := decodeAuthExportBundle([]byte(body))
	require.ErrorContains(t, err, "default does not name")
}

func TestRejectDuplicateObjectKeysAcceptsCanonical(t *testing.T) {
	require.NoError(t, rejectDuplicateObjectKeys([]byte(validBundleJSON())))
}

func TestEncodeAuthExportBundleProducesCanonicalJSON(t *testing.T) {
	bundle, err := decodeAuthExportBundle([]byte(validBundleJSON()))
	require.NoError(t, err)
	body, err := encodeAuthExportBundle(bundle)
	require.NoError(t, err)
	// Re-decoding the canonical output must not surface duplicate-key errors and
	// must round-trip the documented fields.
	roundTrip, err := decodeAuthExportBundle(body)
	require.NoError(t, err)
	require.Equal(t, bundle.DefaultUserID, roundTrip.DefaultUserID)
	require.Len(t, roundTrip.Accounts, len(bundle.Accounts))
	_ = json.Marshal // keep encoding/json referenced by future assertions
}
