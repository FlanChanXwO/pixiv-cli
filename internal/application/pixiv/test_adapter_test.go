package pixiv

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
)

type testPixivRepository struct{ db *authdb.DB }

func (r testPixivRepository) SavePixivCredential(ctx context.Context, account PixivAccountRecord) error {
	return r.db.SavePixivCredential(ctx, authdb.PixivAccount{UserID: account.UserID, SortOrder: account.SortOrder, Username: account.Username, RefreshToken: account.RefreshToken, CredentialRevision: account.CredentialRevision, PremiumStatus: account.PremiumStatus, PremiumCheckedAt: account.PremiumCheckedAt, PoolFrozenUntil: account.PoolFrozenUntil, PoolLastSelected: account.PoolLastSelected, Schedulable: account.Schedulable})
}
func (r testPixivRepository) SavePixivCredentials(ctx context.Context, accounts []PixivAccountRecord) error {
	converted := make([]authdb.PixivAccount, 0, len(accounts))
	for _, account := range accounts {
		converted = append(converted, authdb.PixivAccount{UserID: account.UserID, SortOrder: account.SortOrder, Username: account.Username, RefreshToken: account.RefreshToken, CredentialRevision: account.CredentialRevision, PremiumStatus: account.PremiumStatus, PremiumCheckedAt: account.PremiumCheckedAt, PoolFrozenUntil: account.PoolFrozenUntil, PoolLastSelected: account.PoolLastSelected, Schedulable: account.Schedulable})
	}
	return r.db.SavePixivCredentials(ctx, converted)
}
func (r testPixivRepository) UpdatePixivMetadata(ctx context.Context, userID int64, username string, premium *bool, checkedAt *int64) error {
	return r.db.UpdatePixivMetadata(ctx, userID, username, premium, checkedAt)
}
func (r testPixivRepository) RotatePixivCredentials(ctx context.Context, userID, revision int64, token []byte) error {
	return r.db.RotatePixivCredentials(ctx, userID, revision, token)
}
func (r testPixivRepository) ListPixiv(ctx context.Context) ([]PixivAccountRecord, error) {
	accounts, err := r.db.ListPixiv(ctx)
	if err != nil {
		return nil, err
	}
	return testPixivRecords(accounts), nil
}
func (r testPixivRepository) GetPixiv(ctx context.Context, userID int64) (PixivAccountRecord, error) {
	account, err := r.db.GetPixiv(ctx, userID)
	if err != nil {
		return PixivAccountRecord{}, err
	}
	return testPixivRecord(account), nil
}
func (r testPixivRepository) RemovePixiv(ctx context.Context, userID int64) error {
	return r.db.RemovePixiv(ctx, userID)
}
func (r testPixivRepository) SetPixivSchedulable(ctx context.Context, userIDs []int64, schedulable bool) error {
	return r.db.SetPixivSchedulable(ctx, userIDs, schedulable)
}
func (r testPixivRepository) SetAllPixivSchedulable(ctx context.Context, schedulable bool) error {
	return r.db.SetAllPixivSchedulable(ctx, schedulable)
}
func (r testPixivRepository) ListPixivPoolStatus(ctx context.Context, now int64) (PixivPoolStatusRecord, error) {
	status, err := r.db.ListPixivPoolStatus(ctx, now)
	if err != nil {
		return PixivPoolStatusRecord{}, err
	}
	out := PixivPoolStatusRecord{EarliestFrozenUntil: cloneTestInt64(status.EarliestFrozenUntil)}
	for _, account := range status.Accounts {
		out.Accounts = append(out.Accounts, PixivPoolAccountRecord{UserID: account.UserID, SortOrder: account.SortOrder, Schedulable: account.Schedulable, PoolFrozenUntil: cloneTestInt64(account.PoolFrozenUntil), PoolLastSelected: account.PoolLastSelected, Eligible: account.Eligible})
	}
	return out, nil
}
func (r testPixivRepository) SelectPooledPixiv(ctx context.Context, now int64, strategy PoolStrategy, attempted []int64) (PixivAccountRecord, error) {
	account, err := r.db.SelectPooledPixiv(ctx, now, authdb.PoolStrategy(strategy), attempted)
	if err != nil {
		var selectionErr *authdb.PoolSelectionError
		if errors.As(err, &selectionErr) {
			return PixivAccountRecord{}, &PoolSelectionError{Kind: string(selectionErr.Kind), EarliestFrozenUntil: cloneTestInt64(selectionErr.EarliestFrozenUntil)}
		}
		return PixivAccountRecord{}, err
	}
	return testPixivRecord(account), nil
}
func (r testPixivRepository) FreezePooledPixiv(ctx context.Context, userID, until int64) error {
	return r.db.FreezePooledPixiv(ctx, userID, until)
}

type testPixivDefaults struct{}

func (testPixivDefaults) ReadPixivDefaultUserID() (int64, bool, error) {
	return config.ReadPixivDefaultUserID()
}
func (testPixivDefaults) SetPixivDefaultUserID(userID int64) error {
	return config.SetPixivDefaultUserID(userID)
}
func (testPixivDefaults) ClearPixivDefaultUserID() error { return config.ClearPixivDefaultUserID() }

func testPixivRecord(account authdb.PixivAccount) PixivAccountRecord {
	return PixivAccountRecord{UserID: account.UserID, SortOrder: account.SortOrder, Username: account.Username, RefreshToken: account.RefreshToken, CredentialRevision: account.CredentialRevision, PremiumStatus: account.PremiumStatus, PremiumCheckedAt: account.PremiumCheckedAt, PoolFrozenUntil: account.PoolFrozenUntil, PoolLastSelected: account.PoolLastSelected, Schedulable: account.Schedulable}
}
func testPixivRecords(accounts []authdb.PixivAccount) []PixivAccountRecord {
	out := make([]PixivAccountRecord, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, testPixivRecord(account))
	}
	return out
}
func cloneTestInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
