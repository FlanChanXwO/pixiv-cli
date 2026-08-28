package pixiv_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	sdkpixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

type pixivRoundTripper func(*http.Request) (*http.Response, error)

func (f pixivRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func pixivJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type pixivTestDefaults struct {
	userID   int64
	ok       bool
	readErr  error
	setErr   error
	clearErr error
}

func (d *pixivTestDefaults) ReadPixivDefaultUserID() (int64, bool, error) {
	return d.userID, d.ok, d.readErr
}

func (d *pixivTestDefaults) SetPixivDefaultUserID(userID int64) error {
	if d.setErr != nil {
		return d.setErr
	}
	d.userID, d.ok = userID, true
	return nil
}

func (d *pixivTestDefaults) ClearPixivDefaultUserID() error {
	if d.clearErr != nil {
		return d.clearErr
	}
	d.userID, d.ok = 0, false
	return nil
}

type pixivRotationCall struct {
	userID           int64
	expectedRevision int64
	refreshToken     []byte
}

type pixivTestRepository struct {
	accounts   map[int64]accountpixiv.Account
	rotateErr  error
	rotateCall *pixivRotationCall
}

func newPixivTestRepository(accounts ...accountpixiv.Account) *pixivTestRepository {
	repository := &pixivTestRepository{accounts: make(map[int64]accountpixiv.Account, len(accounts))}
	for _, account := range accounts {
		repository.accounts[account.UserID] = account
	}
	return repository
}

func pixivAccountFixture(userID int64, username, token string, sortOrder int64, schedulable bool) accountpixiv.Account {
	account := accountpixiv.New(userID, username, []byte(token))
	account.SortOrder, account.Schedulable = sortOrder, schedulable
	return account
}

func (r *pixivTestRepository) SavePixivCredential(_ context.Context, account accountpixiv.Account) error {
	if account.UserID <= 0 || !account.HasRefreshToken() {
		return errors.New("invalid pixiv account")
	}
	if existing, ok := r.accounts[account.UserID]; ok {
		account.SortOrder = existing.SortOrder
		account.CredentialRevision = existing.CredentialRevision + 1
	} else {
		if account.SortOrder <= 0 {
			account.SortOrder = int64(len(r.accounts) + 1)
		}
		if account.CredentialRevision <= 0 {
			account.CredentialRevision = 1
		}
		account.Schedulable = true
	}
	r.accounts[account.UserID] = clonePixivAccount(account)
	return nil
}

func (r *pixivTestRepository) SavePixivCredentials(ctx context.Context, accounts []accountpixiv.Account) error {
	seen := make(map[int64]struct{}, len(accounts))
	for _, account := range accounts {
		if _, ok := seen[account.UserID]; ok {
			return fmt.Errorf("duplicate pixiv account %d", account.UserID)
		}
		seen[account.UserID] = struct{}{}
		if err := r.SavePixivCredential(ctx, account); err != nil {
			return err
		}
	}
	return nil
}

func (r *pixivTestRepository) UpdatePixivMetadata(_ context.Context, userID int64, username string, premium *bool, checkedAt *int64) error {
	account, ok := r.accounts[userID]
	if !ok {
		return accountpixiv.ErrNotFound
	}
	account.Username = username
	account.PremiumStatus = cloneBool(premium)
	account.PremiumCheckedAt = cloneInt64(checkedAt)
	r.accounts[userID] = clonePixivAccount(account)
	return nil
}

func (r *pixivTestRepository) RotatePixivCredentials(_ context.Context, userID, expectedRevision int64, refreshToken []byte) error {
	r.rotateCall = &pixivRotationCall{
		userID:           userID,
		expectedRevision: expectedRevision,
		refreshToken:     append([]byte(nil), refreshToken...),
	}
	if r.rotateErr != nil {
		return r.rotateErr
	}
	account, ok := r.accounts[userID]
	if !ok {
		return accountpixiv.ErrNotFound
	}
	if account.CredentialRevision != expectedRevision {
		return accountpixiv.ErrCredentialConflict
	}
	updated := accountpixiv.New(account.UserID, account.Username, refreshToken)
	updated.SortOrder = account.SortOrder
	updated.CredentialRevision = account.CredentialRevision + 1
	updated.PremiumStatus = cloneBool(account.PremiumStatus)
	updated.PremiumCheckedAt = cloneInt64(account.PremiumCheckedAt)
	updated.PoolFrozenUntil = cloneInt64(account.PoolFrozenUntil)
	updated.PoolLastSelected = account.PoolLastSelected
	updated.Schedulable = account.Schedulable
	r.accounts[userID] = updated
	return nil
}

func (r *pixivTestRepository) ListPixiv(_ context.Context) ([]accountpixiv.Account, error) {
	accounts := make([]accountpixiv.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		accounts = append(accounts, clonePixivAccount(account))
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].SortOrder < accounts[j].SortOrder })
	return accounts, nil
}

func (r *pixivTestRepository) GetPixiv(_ context.Context, userID int64) (accountpixiv.Account, error) {
	account, ok := r.accounts[userID]
	if !ok {
		return accountpixiv.Account{}, accountpixiv.ErrNotFound
	}
	return clonePixivAccount(account), nil
}

func (r *pixivTestRepository) RemovePixiv(_ context.Context, userID int64) error {
	if _, ok := r.accounts[userID]; !ok {
		return accountpixiv.ErrNotFound
	}
	delete(r.accounts, userID)
	return nil
}

func (r *pixivTestRepository) SetPixivSchedulable(_ context.Context, userIDs []int64, schedulable bool) error {
	for _, userID := range userIDs {
		if _, ok := r.accounts[userID]; !ok {
			return accountpixiv.ErrNotFound
		}
	}
	for _, userID := range userIDs {
		account := r.accounts[userID]
		account.Schedulable = schedulable
		r.accounts[userID] = account
	}
	return nil
}

func (r *pixivTestRepository) SetAllPixivSchedulable(_ context.Context, schedulable bool) error {
	for userID, account := range r.accounts {
		account.Schedulable = schedulable
		r.accounts[userID] = account
	}
	return nil
}

func (r *pixivTestRepository) ListPixivPoolStatus(ctx context.Context, _ int64) (accountpixiv.PoolStatus, error) {
	accounts, err := r.ListPixiv(ctx)
	if err != nil {
		return accountpixiv.PoolStatus{}, err
	}
	status := accountpixiv.PoolStatus{Accounts: make([]accountpixiv.PoolCandidate, 0, len(accounts))}
	for _, account := range accounts {
		candidate := accountpixiv.PoolCandidate{
			UserID:           account.UserID,
			SortOrder:        account.SortOrder,
			Schedulable:      account.Schedulable,
			PoolFrozenUntil:  cloneInt64(account.PoolFrozenUntil),
			PoolLastSelected: account.PoolLastSelected,
		}
		candidate.Eligible = candidate.Schedulable && candidate.PoolFrozenUntil == nil
		status.Accounts = append(status.Accounts, candidate)
		if candidate.PoolFrozenUntil != nil && (status.EarliestFrozenUntil == nil || *candidate.PoolFrozenUntil < *status.EarliestFrozenUntil) {
			status.EarliestFrozenUntil = cloneInt64(candidate.PoolFrozenUntil)
		}
	}
	return status, nil
}

func (r *pixivTestRepository) SelectPixiv(context.Context, int64, []int64, accountpixiv.Chooser) (accountpixiv.Account, error) {
	return accountpixiv.Account{}, errors.New("select not implemented in test repository")
}

func (r *pixivTestRepository) Freeze(_ context.Context, userID, frozenUntil int64) error {
	account, ok := r.accounts[userID]
	if !ok {
		return accountpixiv.ErrNotFound
	}
	account.PoolFrozenUntil = &frozenUntil
	r.accounts[userID] = account
	return nil
}

func clonePixivAccount(account accountpixiv.Account) accountpixiv.Account {
	cloned := accountpixiv.New(account.UserID, account.Username, account.RefreshTokenCopy())
	cloned.SortOrder = account.SortOrder
	cloned.CredentialRevision = account.CredentialRevision
	cloned.PremiumStatus = cloneBool(account.PremiumStatus)
	cloned.PremiumCheckedAt = cloneInt64(account.PremiumCheckedAt)
	cloned.PoolFrozenUntil = cloneInt64(account.PoolFrozenUntil)
	cloned.PoolLastSelected = account.PoolLastSelected
	cloned.CreatedAt = account.CreatedAt
	cloned.UpdatedAt = account.UpdatedAt
	cloned.Schedulable = account.Schedulable
	return cloned
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func newPixivTestService(repo *pixivTestRepository, defaults ...*pixivTestDefaults) *accountpixiv.Service {
	if len(defaults) == 0 {
		defaults = []*pixivTestDefaults{{}}
	}
	return accountpixiv.NewService(repo, defaults[0])
}

func TestRemoveAccountClearsExplicitDefaultThenRemoves(t *testing.T) {
	repo := newPixivTestRepository(pixivAccountFixture(42, "first", "token", 1, true))
	defaults := &pixivTestDefaults{userID: 42, ok: true}
	service := newPixivTestService(repo, defaults)

	err := service.RemoveAccount(context.Background(), 42)
	require.NoError(t, err)
	_, getErr := repo.GetPixiv(context.Background(), 42)
	require.ErrorIs(t, getErr, accountpixiv.ErrNotFound)
	require.False(t, defaults.ok, "explicit default cleared so first remaining account becomes implicit default")
}

func TestRemoveAccountDefaultClearFailureAbortsRemoval(t *testing.T) {
	repo := newPixivTestRepository(pixivAccountFixture(42, "first", "token", 1, true))
	defaults := &pixivTestDefaults{userID: 42, ok: true, clearErr: errors.New("clear failed")}
	service := newPixivTestService(repo, defaults)

	err := service.RemoveAccount(context.Background(), 42)
	require.ErrorContains(t, err, "clear failed")
	_, getErr := repo.GetPixiv(context.Background(), 42)
	require.NoError(t, getErr, "removal must not proceed when default cannot be cleared")
	require.True(t, defaults.ok, "default preserved on clear failure")
}

func TestVerifyPixivAccountIdentity(t *testing.T) {
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
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := accountpixiv.VerifyAccountIdentity(tc.selected, tc.authenticated)
			if tc.wantError {
				require.Error(t, err)
				require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
				require.NotContains(t, err.Error(), "42")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPixivAutoDefaultMarksSmallestSortOrder(t *testing.T) {
	repo := newPixivTestRepository(
		pixivAccountFixture(42, "second", "token-2", 2, true),
		pixivAccountFixture(7, "first", "token-1", 1, true),
	)
	service := newPixivTestService(repo)

	accounts, err := service.ListAccounts(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.True(t, accounts[0].Default)
	require.False(t, accounts[1].Default)
	require.Equal(t, int64(7), accounts[0].UserID)
}

func TestPixivPoolManagementPersistsNonSecretStatus(t *testing.T) {
	repo := newPixivTestRepository(
		pixivAccountFixture(42, "first", "token-42", 1, true),
		pixivAccountFixture(43, "second", "token-43", 2, true),
	)
	service := newPixivTestService(repo)
	ctx := context.Background()

	require.NoError(t, service.SetPoolSchedulable(ctx, []int64{42}, false))
	accounts, err := service.ListAccounts(ctx)
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.False(t, accounts[0].Schedulable)
	require.False(t, accounts[0].Eligible)
	require.True(t, accounts[1].Schedulable)
	require.True(t, accounts[1].Eligible)

	require.NoError(t, service.SetAllPoolSchedulable(ctx, true))
	status, err := service.PoolStatus(ctx)
	require.NoError(t, err)
	require.Len(t, status.Accounts, 2)
	for _, account := range status.Accounts {
		require.True(t, account.Schedulable)
		require.True(t, account.Eligible)
	}
}

func TestPixivDefaultReadErrorIsReturnedBeforeRemoval(t *testing.T) {
	repo := newPixivTestRepository(pixivAccountFixture(42, "first", "token", 0, false))
	defaults := &pixivTestDefaults{readErr: errors.New("default read failed")}
	service := newPixivTestService(repo, defaults)

	err := service.RemoveAccount(context.Background(), 42)
	require.ErrorContains(t, err, "default read failed")
	_, getErr := repo.GetPixiv(context.Background(), 42)
	require.NoError(t, getErr)
}

func TestRotateCredentialUsesRevisionCAS(t *testing.T) {
	repo := newPixivTestRepository()
	account := accountpixiv.New(42, "first", []byte("old-token"))
	account.CredentialRevision = 2
	repo.accounts[42] = account

	err := accountpixiv.RotateCredential(context.Background(), repo, 42, 42, 1, []byte("new-token"))
	require.ErrorIs(t, err, accountpixiv.ErrCredentialConflict)
	require.Equal(t, int64(1), repo.rotateCall.expectedRevision)

	err = accountpixiv.RotateCredential(context.Background(), repo, 42, 42, 2, []byte("new-token"))
	require.NoError(t, err)
	updated, err := repo.GetPixiv(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, int64(3), updated.CredentialRevision)
	require.Equal(t, "new-token", string(updated.RefreshTokenCopy()))
}

func TestRotateCredentialRejectsIdentityMismatchBeforeRepository(t *testing.T) {
	repo := newPixivTestRepository()
	err := accountpixiv.RotateCredential(context.Background(), repo, 42, 43, 1, []byte("new-token"))
	require.Error(t, err)
	require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
	require.Nil(t, repo.rotateCall)
}

func TestImportAndRestoreValidateCredentialInputs(t *testing.T) {
	repo := newPixivTestRepository()
	service := newPixivTestService(repo)

	_, err := service.ImportAccount(context.Background(), "  ", false)
	require.EqualError(t, err, "pixiv refresh token is required")
	require.Empty(t, repo.accounts)

	err = service.RestoreAccount(context.Background(), accountpixiv.AccountSummary{UserID: 0}, "token", false)
	require.EqualError(t, err, "pixiv refresh token is required")
	err = service.RestoreAccount(context.Background(), accountpixiv.AccountSummary{UserID: 42}, "  ", false)
	require.EqualError(t, err, "pixiv refresh token is required")
}

func TestRestoreAccountPreservesExistingDefault(t *testing.T) {
	repo := newPixivTestRepository()
	defaults := &pixivTestDefaults{userID: 7, ok: true}
	service := newPixivTestService(repo, defaults)

	err := service.RestoreAccount(context.Background(), accountpixiv.AccountSummary{UserID: 42, Username: "restored"}, "restored-token", true)
	require.NoError(t, err)
	require.Equal(t, int64(7), defaults.userID)
	require.True(t, defaults.ok)
}

func TestRestoreAccountsRejectsInvalidInputs(t *testing.T) {
	repo := newPixivTestRepository()
	service := newPixivTestService(repo)

	_, err := service.RestoreAccounts(context.Background(), nil)
	require.ErrorContains(t, err, "no accounts")
	_, err = service.RestoreAccounts(context.Background(), []accountpixiv.RestoreAccountInput{{Account: accountpixiv.AccountSummary{UserID: 0}, RefreshToken: "token"}})
	require.EqualError(t, err, "pixiv refresh token is required")
	_, err = service.RestoreAccounts(context.Background(), []accountpixiv.RestoreAccountInput{{Account: accountpixiv.AccountSummary{UserID: 42}, RefreshToken: "  "}})
	require.EqualError(t, err, "pixiv refresh token is required")
	require.Empty(t, repo.accounts, "validation must occur before any write")
}

func TestRestoreAccountsReportsReplacementsAndAdoptsBundleDefaultOnEmptyStore(t *testing.T) {
	repo := newPixivTestRepository(pixivAccountFixture(7, "old-seven", "old-token", 1, true))
	defaults := &pixivTestDefaults{}
	service := newPixivTestService(repo, defaults)

	inputs := []accountpixiv.RestoreAccountInput{
		{Account: accountpixiv.AccountSummary{UserID: 7, Username: "seven"}, RefreshToken: "new-seven", IsBundleDefault: false},
		{Account: accountpixiv.AccountSummary{UserID: 42, Username: "forty-two"}, RefreshToken: "forty-token", IsBundleDefault: true},
	}
	result, err := service.RestoreAccounts(context.Background(), inputs)
	require.NoError(t, err)
	require.Equal(t, int64(42), result.ResultingDefault, "bundle default adopted when store had no default")
	require.Equal(t, int64(42), defaults.userID)
	require.True(t, defaults.ok)
	require.Len(t, result.Accounts, 2)
	for _, outcome := range result.Accounts {
		if outcome.Account.UserID == 7 {
			require.True(t, outcome.IsReplacement, "pre-existing UID reported as replacement")
		} else {
			require.False(t, outcome.IsReplacement, "new UID reported as added")
		}
	}
	updated, err := repo.GetPixiv(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "new-seven", string(updated.RefreshTokenCopy()))
}

func TestRestoreAccountsPreservesExistingDefault(t *testing.T) {
	repo := newPixivTestRepository(pixivAccountFixture(7, "seven", "seven-token", 1, true))
	defaults := &pixivTestDefaults{userID: 7, ok: true}
	service := newPixivTestService(repo, defaults)

	inputs := []accountpixiv.RestoreAccountInput{
		{Account: accountpixiv.AccountSummary{UserID: 42, Username: "forty-two"}, RefreshToken: "forty-token", IsBundleDefault: true},
	}
	result, err := service.RestoreAccounts(context.Background(), inputs)
	require.NoError(t, err)
	require.Equal(t, int64(7), result.ResultingDefault, "existing local default preserved; bundle default ignored")
	require.Equal(t, int64(7), defaults.userID)
}

func TestRestoreAccountsAdoptsFirstAccountWhenBundleDefaultAbsent(t *testing.T) {
	repo := newPixivTestRepository()
	service := newPixivTestService(repo)

	inputs := []accountpixiv.RestoreAccountInput{
		{Account: accountpixiv.AccountSummary{UserID: 42, Username: "forty-two"}, RefreshToken: "forty-token", IsBundleDefault: false},
		{Account: accountpixiv.AccountSummary{UserID: 7, Username: "seven"}, RefreshToken: "seven-token", IsBundleDefault: false},
	}
	result, err := service.RestoreAccounts(context.Background(), inputs)
	require.NoError(t, err)
	require.Equal(t, int64(42), result.ResultingDefault, "first account becomes default when bundle has none")
}

func TestExportReadsStoredCredentialThroughDefensiveCopy(t *testing.T) {
	repo := newPixivTestRepository(pixivAccountFixture(42, "first", "token", 1, true))
	service := newPixivTestService(repo, &pixivTestDefaults{userID: 42, ok: true})

	token, err := service.ExportRefreshToken(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "token", token)
	accounts, err := service.AccountsWithTokens(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, "token", accounts[0].RefreshToken())
}

func TestImportAccountPassesSDKOptionsAndPreservesPoolState(t *testing.T) {
	repo := newPixivTestRepository()
	existing := accountpixiv.New(42, "old", []byte("old-token"))
	existing.SortOrder = 1
	existing.Schedulable = false
	frozenUntil := int64(4_000_000_000)
	existing.PoolFrozenUntil = &frozenUntil
	repo.accounts[42] = existing
	service := newPixivTestService(repo)

	called := false
	client := &http.Client{Transport: pixivRoundTripper(func(request *http.Request) (*http.Response, error) {
		called = true
		require.Equal(t, "POST", request.Method)
		return pixivJSONResponse(`{"access_token":"access","refresh_token":"rotated","user":{"id":"42","name":"alice"}}`), nil
	})}
	summary, err := service.ImportAccountWith(context.Background(), "token", false, sdkpixiv.Options{HTTPClient: client})
	require.NoError(t, err)
	require.True(t, called, "the caller-provided SDK HTTP client was not used")
	require.Equal(t, int64(42), summary.UserID)
	require.False(t, summary.Schedulable)
	require.False(t, summary.Eligible)
	require.True(t, summary.PoolStatusKnown)
}

func TestCheckAccountRejectsCredentialIdentityMismatchBeforeRotation(t *testing.T) {
	repo := newPixivTestRepository()
	account := accountpixiv.New(42, "local", []byte("local-token"))
	account.CredentialRevision = 3
	repo.accounts[42] = account
	service := newPixivTestService(repo)
	client := &http.Client{Transport: pixivRoundTripper(func(*http.Request) (*http.Response, error) {
		return pixivJSONResponse(`{"access_token":"access","refresh_token":"rotated","user":{"id":"43","name":"foreign"}}`), nil
	})}

	_, err := service.CheckAccountWith(context.Background(), 42, sdkpixiv.Options{HTTPClient: client})
	require.Error(t, err)
	require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
	require.Nil(t, repo.rotateCall)
}

func TestCheckRefreshTokenRejectsCredentialIdentityMismatch(t *testing.T) {
	service := newPixivTestService(newPixivTestRepository())
	client := &http.Client{Transport: pixivRoundTripper(func(*http.Request) (*http.Response, error) {
		return pixivJSONResponse(`{"access_token":"access","refresh_token":"rotated","user":{"id":"43","name":"foreign"}}`), nil
	})}

	_, err := service.CheckRefreshTokenWith(context.Background(), 42, "token", sdkpixiv.Options{HTTPClient: client})
	require.Error(t, err)
	require.Equal(t, sdk.LocalStateError, sdk.ReasonOf(err))
}
