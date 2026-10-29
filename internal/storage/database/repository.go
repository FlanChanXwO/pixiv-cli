package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	fanboxaccount "github.com/FlanChanXwO/pixiv-cli/internal/fanbox/account"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixiv/account"
	"gorm.io/gorm"
)

// SavePixivCredential 保存一次已验证的 Pixiv credential。
// 新账号固定从 revision 1 开始；已有账号在 SQL 内递增 revision，不能由调用方回退。
func (d *DB) SavePixivCredential(ctx context.Context, account pixivaccount.Account) error {
	return d.withLock(ctx, func() error {
		return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return savePixivCredentialTx(ctx, tx, account)
		})
	})
}

// SavePixivCredentials 在一个事务中保存一组首次导入的 Pixiv credentials。
// 迁移调用它保证任一账号失败时不会留下部分提交。
func (d *DB) SavePixivCredentials(ctx context.Context, accounts []pixivaccount.Account) error {
	return d.withLock(ctx, func() error {
		seen := make(map[int64]struct{}, len(accounts))
		for _, account := range accounts {
			if account.UserID <= 0 || !account.HasRefreshToken() {
				return errors.New("database: invalid pixiv account")
			}
			if _, ok := seen[account.UserID]; ok {
				return fmt.Errorf("database: duplicate pixiv account %d", account.UserID)
			}
			seen[account.UserID] = struct{}{}
		}
		return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for _, account := range accounts {
				if err := savePixivCredentialTx(ctx, tx, account); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func savePixivCredentialTx(ctx context.Context, tx *gorm.DB, account pixivaccount.Account) error {
	if account.UserID <= 0 || !account.HasRefreshToken() {
		return errors.New("database: invalid pixiv account")
	}
	now := time.Now().UTC().Unix()
	var existing pixivAccountRow
	err := tx.WithContext(ctx).Select(colSortOrder).Where(colUserID+" = ?", account.UserID).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		sortOrder := account.SortOrder
		if sortOrder <= 0 {
			var maxOrder int64
			if err := tx.WithContext(ctx).Model(&pixivAccountRow{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder).Error; err != nil {
				return err
			}
			sortOrder = maxOrder + 1
		}
		return tx.WithContext(ctx).Create(&pixivAccountRow{
			UserID:             account.UserID,
			SortOrder:          sortOrder,
			Username:           account.Username,
			RefreshToken:       account.RefreshTokenCopy(),
			CredentialRevision: 1,
			PremiumStatus:      boolPtrToIntPtr(account.PremiumStatus),
			PremiumCheckedAt:   account.PremiumCheckedAt,
			PoolFrozenUntil:    account.PoolFrozenUntil,
			PoolLastSelected:   account.PoolLastSelected,
			CreatedAt:          now,
			UpdatedAt:          now,
			Schedulable:        true,
		}).Error
	}
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&pixivAccountRow{}).Where(colUserID+" = ?", account.UserID).
		Updates(map[string]any{
			colUsername:           account.Username,
			colRefreshToken:       account.RefreshTokenCopy(),
			colCredentialRevision: gorm.Expr(colCredentialRevision + " + 1"),
			colUpdatedAt:          now,
		}).Error
}

// UpdatePixivMetadata 更新会员快照等非 credential 元数据，不改变 revision 或 token。
func (d *DB) UpdatePixivMetadata(ctx context.Context, userID int64, username string, premiumStatus *bool, premiumCheckedAt *int64) error {
	return d.withLock(ctx, func() error {
		if userID <= 0 {
			return errors.New("database: invalid pixiv metadata input")
		}
		result := d.orm.WithContext(ctx).Model(&pixivAccountRow{}).Where(colUserID+" = ?", userID).
			Updates(map[string]any{
				colUsername:         username,
				colPremiumStatus:    boolPtrToIntPtr(premiumStatus),
				colPremiumCheckedAt: premiumCheckedAt,
				colUpdatedAt:        time.Now().UTC().Unix(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return pixivaccount.ErrNotFound
		}
		return nil
	})
}

// RotatePixivCredentials 提交一次 refresh token rotation：更新 token 并递增
// credential_revision。expectedRevision 不匹配时返回 ErrCredentialConflict。
func (d *DB) RotatePixivCredentials(ctx context.Context, userID, expectedRevision int64, refreshToken []byte) error {
	return d.withLock(ctx, func() error {
		if userID <= 0 || expectedRevision <= 0 || len(refreshToken) == 0 {
			return errors.New("database: invalid rotation input")
		}
		result := d.orm.WithContext(ctx).Model(&pixivAccountRow{}).
			Where(colUserID+" = ? AND "+colCredentialRevision+" = ?", userID, expectedRevision).
			Updates(map[string]any{
				colRefreshToken:       refreshToken,
				colCredentialRevision: gorm.Expr(colCredentialRevision + " + 1"),
				colUpdatedAt:          time.Now().UTC().Unix(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 0 {
			return nil
		}
		var count int64
		if err := d.orm.WithContext(ctx).Model(&pixivAccountRow{}).Where(colUserID+" = ?", userID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return pixivaccount.ErrNotFound
		}
		return pixivaccount.ErrCredentialConflict
	})
}

// ListPixiv 返回全部 Pixiv 账号，按 sort_order 升序。
func (d *DB) ListPixiv(ctx context.Context) ([]pixivaccount.Account, error) {
	return withDatabaseValue(ctx, d, func() ([]pixivaccount.Account, error) {
		var rows []pixivAccountRow
		if err := d.orm.WithContext(ctx).Order(colSortOrder).Find(&rows).Error; err != nil {
			return nil, err
		}
		accounts := make([]pixivaccount.Account, len(rows))
		for i, row := range rows {
			accounts[i] = row.account()
		}
		return accounts, nil
	})
}

// GetPixiv 返回指定 user_id 的 Pixiv 账号；不存在返回 ErrNotFound。
func (d *DB) GetPixiv(ctx context.Context, userID int64) (pixivaccount.Account, error) {
	return withDatabaseValue(ctx, d, func() (pixivaccount.Account, error) {
		var row pixivAccountRow
		if err := d.orm.WithContext(ctx).Where(colUserID+" = ?", userID).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return pixivaccount.Account{}, pixivaccount.ErrNotFound
			}
			return pixivaccount.Account{}, err
		}
		return row.account(), nil
	})
}

// RemovePixiv 删除一个 Pixiv 账号，其 pool 标记随之自然删除。
func (d *DB) RemovePixiv(ctx context.Context, userID int64) error {
	return d.withLock(ctx, func() error {
		result := d.orm.WithContext(ctx).Where(colUserID+" = ?", userID).Delete(&pixivAccountRow{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return pixivaccount.ErrNotFound
		}
		return nil
	})
}

// SelectPixiv 在一个短事务内加载候选快照，并把纯 chooser 的结果验证后提交
// 为 pool_last_selected marker。chooser 不接触 SQL；attemptedUserIDs 只属于当前
// operation，不会写入数据库；过期冻结在选择时清理。
func (d *DB) SelectPixiv(ctx context.Context, now int64, attemptedUserIDs []int64, chooser pixivaccount.Chooser) (pixivaccount.Account, error) {
	return withDatabaseValue(ctx, d, func() (pixivaccount.Account, error) {
		var selected pixivaccount.Account
		err := d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			candidates, marker, err := loadPoolSnapshot(ctx, tx, now, attemptedUserIDs)
			if err != nil {
				return err
			}
			if chooser == nil {
				return errors.New("database: pixiv pool chooser is required")
			}
			chosenUserID, err := chooser(pixivaccount.PoolSnapshot{
				Candidates:      candidates,
				MarkerUserID:    marker.userID,
				MarkerSortOrder: marker.sortOrder,
			})
			if err != nil {
				return err
			}
			if !containsPoolCandidate(candidates, chosenUserID) {
				return fmt.Errorf("database: pixiv pool chooser selected uid %d outside transaction snapshot", chosenUserID)
			}
			selected, err = commitPoolSelection(ctx, tx, chosenUserID, now)
			return err
		})
		return selected, err
	})
}

func loadPoolSnapshot(ctx context.Context, tx *gorm.DB, now int64, attemptedUserIDs []int64) ([]pixivaccount.PoolCandidate, poolMarker, error) {
	if err := clearExpiredPoolFreezes(ctx, tx, now); err != nil {
		return nil, poolMarker{}, err
	}
	if err := requireSchedulableAccounts(ctx, tx, now); err != nil {
		return nil, poolMarker{}, err
	}
	rows, err := queryPoolCandidates(ctx, tx, now, attemptedUserIDs)
	if err != nil {
		return nil, poolMarker{}, err
	}
	candidates := make([]pixivaccount.PoolCandidate, len(rows))
	for i, row := range rows {
		candidates[i] = row.candidate(now)
	}
	if len(candidates) == 0 {
		earliest, err := earliestFrozenInTx(ctx, tx, now)
		if err != nil {
			return nil, poolMarker{}, err
		}
		return nil, poolMarker{}, &pixivaccount.PoolSelectionError{Kind: pixivaccount.PoolSelectionExhausted, EarliestFrozenUntil: earliest}
	}
	marker, err := readPoolMarker(ctx, tx)
	if err != nil {
		return nil, poolMarker{}, err
	}
	return candidates, marker, nil
}

func clearExpiredPoolFreezes(ctx context.Context, tx *gorm.DB, now int64) error {
	return tx.Model(&pixivAccountRow{}).
		Where(colPoolFrozenUntil+" IS NOT NULL AND "+colPoolFrozenUntil+" <= ?", now).
		Update(colPoolFrozenUntil, nil).Error
}

func requireSchedulableAccounts(ctx context.Context, tx *gorm.DB, now int64) error {
	var total, schedulable, eligible int64
	if err := tx.Model(&pixivAccountRow{}).Count(&total).Error; err != nil {
		return err
	}
	if total == 0 {
		return &pixivaccount.PoolSelectionError{Kind: pixivaccount.PoolSelectionNoLocalAccount}
	}
	if err := tx.Model(&pixivAccountRow{}).Where(colSchedulable+" = ?", true).Count(&schedulable).Error; err != nil {
		return err
	}
	if schedulable == 0 {
		return &pixivaccount.PoolSelectionError{Kind: pixivaccount.PoolSelectionNoSchedulable}
	}
	eligibleScope, eligibleArgs := poolScope(now)
	if err := tx.Model(&pixivAccountRow{}).Where(eligibleScope, eligibleArgs...).Count(&eligible).Error; err != nil {
		return err
	}
	if eligible == 0 {
		earliest, err := earliestFrozenInTx(ctx, tx, now)
		if err != nil {
			return err
		}
		return &pixivaccount.PoolSelectionError{Kind: pixivaccount.PoolSelectionAllFrozen, EarliestFrozenUntil: earliest}
	}
	return nil
}

func queryPoolCandidates(ctx context.Context, tx *gorm.DB, now int64, attemptedUserIDs []int64) ([]poolCandidateRow, error) {
	scope, args := poolScope(now)
	query := tx.WithContext(ctx).Table(tablePixivAccount).Where(scope, args...)
	if len(attemptedUserIDs) > 0 {
		query = query.Where(colUserID+" NOT IN ?", attemptedUserIDs)
	}
	var rows []poolCandidateRow
	if err := query.Select(colUserID, colSortOrder, colSchedulable, colPoolFrozenUntil, colPoolLastSelected).
		Order(colSortOrder).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type poolMarker struct {
	userID    *int64
	sortOrder *int64
}

func readPoolMarker(ctx context.Context, tx *gorm.DB) (poolMarker, error) {
	var row poolCandidateRow
	err := tx.WithContext(ctx).Table(tablePixivAccount).Select(colUserID, colSortOrder).
		Where(colPoolLastSelected+" = ?", true).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return poolMarker{}, nil
	}
	if err != nil {
		return poolMarker{}, err
	}
	return poolMarker{userID: &row.UserID, sortOrder: &row.SortOrder}, nil
}

func containsPoolCandidate(candidates []pixivaccount.PoolCandidate, userID int64) bool {
	for _, candidate := range candidates {
		if candidate.UserID == userID {
			return true
		}
	}
	return false
}

func commitPoolSelection(ctx context.Context, tx *gorm.DB, chosenUserID, now int64) (pixivaccount.Account, error) {
	var selectedRow pixivAccountRow
	if err := tx.WithContext(ctx).Where(colUserID+" = ?", chosenUserID).Take(&selectedRow).Error; err != nil {
		return pixivaccount.Account{}, err
	}
	if err := tx.Model(&pixivAccountRow{}).Where(colPoolLastSelected+" = ?", true).
		Update(colPoolLastSelected, false).Error; err != nil {
		return pixivaccount.Account{}, err
	}
	if err := tx.Model(&pixivAccountRow{}).Where(colUserID+" = ?", chosenUserID).
		Updates(map[string]any{colPoolLastSelected: true, colUpdatedAt: now}).Error; err != nil {
		return pixivaccount.Account{}, err
	}
	selected := selectedRow.account()
	selected.PoolLastSelected = true
	return selected, nil
}

// Freeze 记录一个账号的有效冻结时间。
func (d *DB) Freeze(ctx context.Context, userID, frozenUntil int64) error {
	return d.withLock(ctx, func() error {
		result := d.orm.WithContext(ctx).Model(&pixivAccountRow{}).Where(colUserID+" = ?", userID).
			Updates(map[string]any{
				colPoolFrozenUntil: gorm.Expr("CASE WHEN "+colPoolFrozenUntil+" IS NULL OR "+colPoolFrozenUntil+" < ? THEN ? ELSE "+colPoolFrozenUntil+" END", frozenUntil, frozenUntil),
				colUpdatedAt:       time.Now().UTC().Unix(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return pixivaccount.ErrNotFound
		}
		return nil
	})
}

// SaveFanboxCredential 保存一次已验证的 FANBOX credential。
// 新账号固定从 revision 1 开始；已有账号在 SQL 内递增 revision。
func (d *DB) SaveFanboxCredential(ctx context.Context, account fanboxaccount.Account) error {
	return d.withLock(ctx, func() error {
		if account.UserID <= 0 || !account.HasSession() || account.ValidatedAt <= 0 {
			return errors.New("database: invalid fanbox account")
		}
		now := time.Now().UTC().Unix()
		return d.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var existing fanboxAccountRow
			err := tx.Select(colSortOrder).Where(colUserID+" = ?", account.UserID).Take(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				sortOrder := account.SortOrder
				if sortOrder <= 0 {
					var maxOrder int64
					if err := tx.Model(&fanboxAccountRow{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder).Error; err != nil {
						return err
					}
					sortOrder = maxOrder + 1
				}
				return tx.Create(&fanboxAccountRow{
					UserID:             account.UserID,
					SortOrder:          sortOrder,
					DisplayName:        account.DisplayName,
					CreatorID:          account.CreatorID,
					SessionID:          account.SessionIDCopy(),
					CredentialRevision: 1,
					ValidatedAt:        account.ValidatedAt,
					CreatedAt:          now,
					UpdatedAt:          now,
				}).Error
			}
			if err != nil {
				return err
			}
			return tx.Model(&fanboxAccountRow{}).Where(colUserID+" = ?", account.UserID).
				Updates(map[string]any{
					colDisplayName:        account.DisplayName,
					colCreatorID:          account.CreatorID,
					colSessionID:          account.SessionIDCopy(),
					colCredentialRevision: gorm.Expr(colCredentialRevision + " + 1"),
					colValidatedAt:        account.ValidatedAt,
					colUpdatedAt:          now,
				}).Error
		})
	})
}

// ListFanbox 返回全部 FANBOX 账号，按 sort_order 升序。
func (d *DB) ListFanbox(ctx context.Context) ([]fanboxaccount.Account, error) {
	return withDatabaseValue(ctx, d, func() ([]fanboxaccount.Account, error) {
		var rows []fanboxAccountRow
		if err := d.orm.WithContext(ctx).Order(colSortOrder).Find(&rows).Error; err != nil {
			return nil, err
		}
		accounts := make([]fanboxaccount.Account, len(rows))
		for i, row := range rows {
			accounts[i] = row.account()
		}
		return accounts, nil
	})
}

// GetFanbox 返回指定 user_id 的 FANBOX 账号；不存在返回 ErrNotFound。
func (d *DB) GetFanbox(ctx context.Context, userID int64) (fanboxaccount.Account, error) {
	return withDatabaseValue(ctx, d, func() (fanboxaccount.Account, error) {
		var row fanboxAccountRow
		if err := d.orm.WithContext(ctx).Where(colUserID+" = ?", userID).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fanboxaccount.Account{}, fanboxaccount.ErrNotFound
			}
			return fanboxaccount.Account{}, err
		}
		return row.account(), nil
	})
}

// RemoveFanbox 删除一个 FANBOX 账号。
func (d *DB) RemoveFanbox(ctx context.Context, userID int64) error {
	return d.withLock(ctx, func() error {
		result := d.orm.WithContext(ctx).Where(colUserID+" = ?", userID).Delete(&fanboxAccountRow{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fanboxaccount.ErrNotFound
		}
		return nil
	})
}

func boolPtrToIntPtr(value *bool) *int64 {
	if value == nil {
		return nil
	}
	if *value {
		one := int64(1)
		return &one
	}
	zero := int64(0)
	return &zero
}

func intPtrToBoolPtr(value *int64) *bool {
	if value == nil {
		return nil
	}
	out := *value == 1
	return &out
}
