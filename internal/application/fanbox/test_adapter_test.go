package fanbox

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
)

type testFanboxRepository struct{ db *authdb.DB }

func (r testFanboxRepository) SaveFanboxCredential(ctx context.Context, account FanboxAccountRecord) error {
	return r.db.SaveFanboxCredential(ctx, authdb.FanboxAccount{UserID: account.UserID, SortOrder: account.SortOrder, DisplayName: account.DisplayName, CreatorID: account.CreatorID, SessionID: account.SessionID, CredentialRevision: account.CredentialRevision, ValidatedAt: account.ValidatedAt})
}
func (r testFanboxRepository) RotateFanboxSession(ctx context.Context, userID, revision int64, session []byte, validatedAt int64) error {
	return r.db.RotateFanboxSession(ctx, userID, revision, session, validatedAt)
}
func (r testFanboxRepository) ListFanbox(ctx context.Context) ([]FanboxAccountRecord, error) {
	accounts, err := r.db.ListFanbox(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FanboxAccountRecord, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, testFanboxRecord(account))
	}
	return out, nil
}
func (r testFanboxRepository) GetFanbox(ctx context.Context, userID int64) (FanboxAccountRecord, error) {
	account, err := r.db.GetFanbox(ctx, userID)
	if err != nil {
		return FanboxAccountRecord{}, err
	}
	return testFanboxRecord(account), nil
}
func (r testFanboxRepository) RemoveFanbox(ctx context.Context, userID int64) error {
	return r.db.RemoveFanbox(ctx, userID)
}

type testFanboxDefaults struct{}

func (testFanboxDefaults) ReadFanboxDefaultUserID() (int64, bool, error) {
	return config.ReadFanboxDefaultUserID()
}
func (testFanboxDefaults) SetFanboxDefaultUserID(userID int64) error {
	return config.SetFanboxDefaultUserID(userID)
}
func (testFanboxDefaults) ClearFanboxDefaultUserID() error { return config.ClearFanboxDefaultUserID() }

func testFanboxRecord(account authdb.FanboxAccount) FanboxAccountRecord {
	return FanboxAccountRecord{UserID: account.UserID, SortOrder: account.SortOrder, DisplayName: account.DisplayName, CreatorID: account.CreatorID, SessionID: account.SessionID, CredentialRevision: account.CredentialRevision, ValidatedAt: account.ValidatedAt}
}
