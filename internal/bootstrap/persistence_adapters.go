package bootstrap

import (
	"context"
	"errors"

	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/persistence/authdb"
)

// pixivPersistenceAdapter 把 SQLite repository DTO 转换为 application/pixiv
// 的端口 DTO。转换集中在 composition root，应用层不感知 authdb 表结构。
type pixivPersistenceAdapter struct{ db *authdb.DB }

func (a pixivPersistenceAdapter) SavePixivCredential(ctx context.Context, account pixivapp.PixivAccountRecord) error {
	return a.db.SavePixivCredential(ctx, toAuthDBPixiv(account))
}

func (a pixivPersistenceAdapter) SavePixivCredentials(ctx context.Context, accounts []pixivapp.PixivAccountRecord) error {
	converted := make([]authdb.PixivAccount, 0, len(accounts))
	for _, account := range accounts {
		converted = append(converted, toAuthDBPixiv(account))
	}
	return a.db.SavePixivCredentials(ctx, converted)
}

func (a pixivPersistenceAdapter) UpdatePixivMetadata(ctx context.Context, userID int64, username string, premium *bool, checkedAt *int64) error {
	return a.db.UpdatePixivMetadata(ctx, userID, username, premium, checkedAt)
}

func (a pixivPersistenceAdapter) RotatePixivCredentials(ctx context.Context, userID, revision int64, token []byte) error {
	return a.db.RotatePixivCredentials(ctx, userID, revision, token)
}

func (a pixivPersistenceAdapter) ListPixiv(ctx context.Context) ([]pixivapp.PixivAccountRecord, error) {
	accounts, err := a.db.ListPixiv(ctx)
	if err != nil {
		return nil, err
	}
	return fromAuthDBPixivList(accounts), nil
}

func (a pixivPersistenceAdapter) GetPixiv(ctx context.Context, userID int64) (pixivapp.PixivAccountRecord, error) {
	account, err := a.db.GetPixiv(ctx, userID)
	if err != nil {
		return pixivapp.PixivAccountRecord{}, err
	}
	return fromAuthDBPixiv(account), nil
}

func (a pixivPersistenceAdapter) RemovePixiv(ctx context.Context, userID int64) error {
	return a.db.RemovePixiv(ctx, userID)
}

func (a pixivPersistenceAdapter) SetPixivSchedulable(ctx context.Context, userIDs []int64, schedulable bool) error {
	return a.db.SetPixivSchedulable(ctx, userIDs, schedulable)
}

func (a pixivPersistenceAdapter) SetAllPixivSchedulable(ctx context.Context, schedulable bool) error {
	return a.db.SetAllPixivSchedulable(ctx, schedulable)
}

func (a pixivPersistenceAdapter) ListPixivPoolStatus(ctx context.Context, now int64) (pixivapp.PixivPoolStatusRecord, error) {
	status, err := a.db.ListPixivPoolStatus(ctx, now)
	if err != nil {
		return pixivapp.PixivPoolStatusRecord{}, err
	}
	out := pixivapp.PixivPoolStatusRecord{EarliestFrozenUntil: cloneInt64(status.EarliestFrozenUntil)}
	out.Accounts = make([]pixivapp.PixivPoolAccountRecord, 0, len(status.Accounts))
	for _, account := range status.Accounts {
		out.Accounts = append(out.Accounts, pixivapp.PixivPoolAccountRecord{
			UserID: account.UserID, SortOrder: account.SortOrder, Schedulable: account.Schedulable,
			PoolFrozenUntil: cloneInt64(account.PoolFrozenUntil), PoolLastSelected: account.PoolLastSelected,
			Eligible: account.Eligible,
		})
	}
	return out, nil
}

func (a pixivPersistenceAdapter) SelectPooledPixiv(ctx context.Context, now int64, strategy pixivapp.PoolStrategy, attempted []int64) (pixivapp.PixivAccountRecord, error) {
	account, err := a.db.SelectPooledPixiv(ctx, now, authdb.PoolStrategy(strategy), attempted)
	if err != nil {
		var selectionErr *authdb.PoolSelectionError
		if errors.As(err, &selectionErr) {
			return pixivapp.PixivAccountRecord{}, &pixivapp.PoolSelectionError{
				Kind: string(selectionErr.Kind), EarliestFrozenUntil: cloneInt64(selectionErr.EarliestFrozenUntil),
			}
		}
		return pixivapp.PixivAccountRecord{}, err
	}
	return fromAuthDBPixiv(account), nil
}

func (a pixivPersistenceAdapter) FreezePooledPixiv(ctx context.Context, userID, until int64) error {
	return a.db.FreezePooledPixiv(ctx, userID, until)
}

func toAuthDBPixiv(account pixivapp.PixivAccountRecord) authdb.PixivAccount {
	return authdb.PixivAccount{
		UserID: account.UserID, SortOrder: account.SortOrder, Username: account.Username,
		RefreshToken: append([]byte(nil), account.RefreshToken...), CredentialRevision: account.CredentialRevision,
		PremiumStatus: account.PremiumStatus, PremiumCheckedAt: account.PremiumCheckedAt,
		PoolFrozenUntil: account.PoolFrozenUntil, PoolLastSelected: account.PoolLastSelected,
		CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt, Schedulable: account.Schedulable,
	}
}

func fromAuthDBPixiv(account authdb.PixivAccount) pixivapp.PixivAccountRecord {
	return pixivapp.PixivAccountRecord{
		UserID: account.UserID, SortOrder: account.SortOrder, Username: account.Username,
		RefreshToken: append([]byte(nil), account.RefreshToken...), CredentialRevision: account.CredentialRevision,
		PremiumStatus: account.PremiumStatus, PremiumCheckedAt: account.PremiumCheckedAt,
		PoolFrozenUntil: account.PoolFrozenUntil, PoolLastSelected: account.PoolLastSelected,
		CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt, Schedulable: account.Schedulable,
	}
}

func fromAuthDBPixivList(accounts []authdb.PixivAccount) []pixivapp.PixivAccountRecord {
	out := make([]pixivapp.PixivAccountRecord, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, fromAuthDBPixiv(account))
	}
	return out
}

// fanboxPersistenceAdapter 把 SQLite FANBOX 记录转换为 application/fanbox DTO。
type fanboxPersistenceAdapter struct{ db *authdb.DB }

func (a fanboxPersistenceAdapter) SaveFanboxCredential(ctx context.Context, account fanboxapp.FanboxAccountRecord) error {
	return a.db.SaveFanboxCredential(ctx, authdb.FanboxAccount{
		UserID: account.UserID, SortOrder: account.SortOrder, DisplayName: account.DisplayName,
		CreatorID: account.CreatorID, SessionID: append([]byte(nil), account.SessionID...),
		CredentialRevision: account.CredentialRevision, ValidatedAt: account.ValidatedAt,
		CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt,
	})
}

func (a fanboxPersistenceAdapter) RotateFanboxSession(ctx context.Context, userID, revision int64, session []byte, validatedAt int64) error {
	return a.db.RotateFanboxSession(ctx, userID, revision, session, validatedAt)
}

func (a fanboxPersistenceAdapter) ListFanbox(ctx context.Context) ([]fanboxapp.FanboxAccountRecord, error) {
	accounts, err := a.db.ListFanbox(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]fanboxapp.FanboxAccountRecord, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, fromAuthDBFanbox(account))
	}
	return out, nil
}

func (a fanboxPersistenceAdapter) GetFanbox(ctx context.Context, userID int64) (fanboxapp.FanboxAccountRecord, error) {
	account, err := a.db.GetFanbox(ctx, userID)
	if err != nil {
		return fanboxapp.FanboxAccountRecord{}, err
	}
	return fromAuthDBFanbox(account), nil
}

func (a fanboxPersistenceAdapter) RemoveFanbox(ctx context.Context, userID int64) error {
	return a.db.RemoveFanbox(ctx, userID)
}

func fromAuthDBFanbox(account authdb.FanboxAccount) fanboxapp.FanboxAccountRecord {
	return fanboxapp.FanboxAccountRecord{
		UserID: account.UserID, SortOrder: account.SortOrder, DisplayName: account.DisplayName,
		CreatorID: account.CreatorID, SessionID: append([]byte(nil), account.SessionID...),
		CredentialRevision: account.CredentialRevision, ValidatedAt: account.ValidatedAt,
		CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt,
	}
}

type configDefaultStore struct {
	store configapp.ConfigFileStore
}

func (s configDefaultStore) ReadPixivDefaultUserID() (int64, bool, error) {
	return s.store.ReadPixivDefaultUserID()
}
func (s configDefaultStore) SetPixivDefaultUserID(userID int64) error {
	return s.store.SetPixivDefaultUserID(userID)
}
func (s configDefaultStore) ClearPixivDefaultUserID() error { return s.store.ClearPixivDefaultUserID() }

func (s configDefaultStore) ReadFanboxDefaultUserID() (int64, bool, error) {
	return s.store.ReadFanboxDefaultUserID()
}
func (s configDefaultStore) SetFanboxDefaultUserID(userID int64) error {
	return s.store.SetFanboxDefaultUserID(userID)
}
func (s configDefaultStore) ClearFanboxDefaultUserID() error {
	return s.store.ClearFanboxDefaultUserID()
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
