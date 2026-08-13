package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	accountfanbox "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
)

const pixivAccountColumns = `user_id, sort_order, username, refresh_token, credential_revision,
	premium_status, premium_checked_at, pool_frozen_until, pool_last_selected, created_at, updated_at, schedulable`

const fanboxAccountColumns = `user_id, sort_order, display_name, creator_id, session_id,
	credential_revision, validated_at, created_at, updated_at`

// SavePixivCredential 保存一次已验证的 Pixiv credential。
// 新账号固定从 revision 1 开始；已有账号在 SQL 内递增 revision，不能由调用方回退。
func (d *DB) SavePixivCredential(ctx context.Context, account accountpixiv.Account) error {
	if account.UserID <= 0 || !account.HasRefreshToken() {
		return errors.New("database: invalid pixiv account")
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := savePixivCredentialTx(ctx, tx, account); err != nil {
		return err
	}
	return tx.Commit()
}

// SavePixivCredentials 在一个事务中保存一组首次导入的 Pixiv credentials。
// 迁移调用它保证任一账号失败时不会留下部分提交。
func (d *DB) SavePixivCredentials(ctx context.Context, accounts []accountpixiv.Account) error {
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
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, account := range accounts {
		if err := savePixivCredentialTx(ctx, tx, account); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func savePixivCredentialTx(ctx context.Context, tx *sql.Tx, account accountpixiv.Account) error {
	now := time.Now().UTC().Unix()
	var sortOrder int64
	err := tx.QueryRowContext(ctx, `SELECT sort_order FROM pixiv_account WHERE user_id = ?`, account.UserID).Scan(&sortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		sortOrder = account.SortOrder
		if sortOrder <= 0 {
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM pixiv_account`).Scan(&sortOrder); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO pixiv_account (`+pixivAccountColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			account.UserID, sortOrder, account.Username, account.RefreshTokenCopy(), 1,
			boolPtrToIntPtr(account.PremiumStatus), account.PremiumCheckedAt, account.PoolFrozenUntil, boolInt(account.PoolLastSelected),
			now, now, 1)
	} else if err != nil {
		return err
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE pixiv_account SET username=?, refresh_token=?,
			credential_revision=credential_revision+1, updated_at=? WHERE user_id=?`,
			account.Username, account.RefreshTokenCopy(), now, account.UserID)
	}
	return err
}

// UpdatePixivMetadata 更新会员快照等非 credential 元数据，不改变 revision 或 token。
func (d *DB) UpdatePixivMetadata(ctx context.Context, userID int64, username string, premiumStatus *bool, premiumCheckedAt *int64) error {
	if userID <= 0 {
		return errors.New("database: invalid pixiv metadata input")
	}
	result, err := d.db.ExecContext(ctx, `UPDATE pixiv_account SET username=?, premium_status=?, premium_checked_at=?, updated_at=? WHERE user_id=?`,
		username, boolPtrToIntPtr(premiumStatus), premiumCheckedAt, time.Now().UTC().Unix(), userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return accountpixiv.ErrNotFound
	}
	return nil
}

// RotatePixivCredentials 提交一次 refresh token rotation：更新 token 并递增
// credential_revision。
func (d *DB) RotatePixivCredentials(ctx context.Context, userID, expectedRevision int64, refreshToken []byte) error {
	if userID <= 0 || expectedRevision <= 0 || len(refreshToken) == 0 {
		return errors.New("database: invalid rotation input")
	}
	result, err := d.db.ExecContext(ctx,
		`UPDATE pixiv_account SET refresh_token=?, credential_revision=credential_revision+1, updated_at=? WHERE user_id=? AND credential_revision=?`,
		refreshToken, time.Now().UTC().Unix(), userID, expectedRevision)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists int
		if err := d.db.QueryRowContext(ctx, `SELECT 1 FROM pixiv_account WHERE user_id=?`, userID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return accountpixiv.ErrNotFound
		} else if err != nil {
			return err
		}
		return accountpixiv.ErrCredentialConflict
	}
	return nil
}

// ListPixiv 返回全部 Pixiv 账号，按 sort_order 升序。
func (d *DB) ListPixiv(ctx context.Context) ([]accountpixiv.Account, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+pixivAccountColumns+` FROM pixiv_account ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPixivAccounts(rows)
}

// GetPixiv 返回指定 user_id 的 Pixiv 账号；不存在返回 ErrNotFound。
func (d *DB) GetPixiv(ctx context.Context, userID int64) (accountpixiv.Account, error) {
	row := d.db.QueryRowContext(ctx, `SELECT `+pixivAccountColumns+` FROM pixiv_account WHERE user_id = ?`, userID)
	account, err := scanPixivAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return accountpixiv.Account{}, accountpixiv.ErrNotFound
	}
	return account, err
}

// RemovePixiv 删除一个 Pixiv 账号，其 pool 标记随之自然删除。
func (d *DB) RemovePixiv(ctx context.Context, userID int64) error {
	result, err := d.db.ExecContext(ctx, `DELETE FROM pixiv_account WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return accountpixiv.ErrNotFound
	}
	return nil
}

// SelectPixiv 在一个短事务内加载候选快照，并把纯 chooser 的结果验证后提交
// 为 pool_last_selected marker。chooser 不接触 SQL；attemptedUserIDs 只属于当前
// operation，不会写入数据库；过期冻结在选择时清理。
func (d *DB) SelectPixiv(ctx context.Context, now int64, attemptedUserIDs []int64, chooser accountpixiv.Chooser) (accountpixiv.Account, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return accountpixiv.Account{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET pool_frozen_until = NULL WHERE pool_frozen_until IS NOT NULL AND pool_frozen_until <= ?`, now); err != nil {
		return accountpixiv.Account{}, err
	}
	var total, schedulable, eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pixiv_account`).Scan(&total); err != nil {
		return accountpixiv.Account{}, err
	}
	if total == 0 {
		return accountpixiv.Account{}, &accountpixiv.PoolSelectionError{Kind: accountpixiv.PoolSelectionNoLocalAccount}
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pixiv_account WHERE schedulable=1`).Scan(&schedulable); err != nil {
		return accountpixiv.Account{}, err
	}
	if schedulable == 0 {
		return accountpixiv.Account{}, &accountpixiv.PoolSelectionError{Kind: accountpixiv.PoolSelectionNoSchedulable}
	}
	eligibleScope, eligibleArgs := poolScope(now, nil)
	if err := tx.QueryRowContext(ctx, `SELECT count(*) `+eligibleScope, eligibleArgs...).Scan(&eligible); err != nil {
		return accountpixiv.Account{}, err
	}
	if eligible == 0 {
		earliest, err := earliestFrozenInTx(ctx, tx, now)
		if err != nil {
			return accountpixiv.Account{}, err
		}
		return accountpixiv.Account{}, &accountpixiv.PoolSelectionError{Kind: accountpixiv.PoolSelectionAllFrozen, EarliestFrozenUntil: earliest}
	}
	scope, args := poolScope(now, attemptedUserIDs)
	rows, err := tx.QueryContext(ctx, `SELECT user_id, sort_order, schedulable, pool_frozen_until, pool_last_selected `+scope+` ORDER BY sort_order`, args...)
	if err != nil {
		return accountpixiv.Account{}, err
	}
	candidates, err := scanPoolCandidates(rows, now)
	if err != nil {
		_ = rows.Close()
		return accountpixiv.Account{}, err
	}
	if err := rows.Close(); err != nil {
		return accountpixiv.Account{}, err
	}
	if len(candidates) == 0 {
		earliest, err := earliestFrozenInTx(ctx, tx, now)
		if err != nil {
			return accountpixiv.Account{}, err
		}
		return accountpixiv.Account{}, &accountpixiv.PoolSelectionError{Kind: accountpixiv.PoolSelectionExhausted, EarliestFrozenUntil: earliest}
	}
	var markerSort sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT sort_order FROM pixiv_account WHERE pool_last_selected=1`).Scan(&markerSort); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return accountpixiv.Account{}, err
	}
	var markerUserID *int64
	var markerSortOrder *int64
	if markerSort.Valid {
		var userID int64
		if err := tx.QueryRowContext(ctx, `SELECT user_id FROM pixiv_account WHERE sort_order=? AND pool_last_selected=1`, markerSort.Int64).Scan(&userID); err != nil {
			return accountpixiv.Account{}, err
		}
		markerUserID = &userID
		markerValue := markerSort.Int64
		markerSortOrder = &markerValue
	}
	if chooser == nil {
		return accountpixiv.Account{}, errors.New("database: pixiv pool chooser is required")
	}
	chosenUserID, err := chooser(accountpixiv.PoolSnapshot{
		Candidates:          candidates,
		MarkerUserID:        markerUserID,
		MarkerSortOrder:     markerSortOrder,
		EarliestFrozenUntil: nil,
	})
	if err != nil {
		return accountpixiv.Account{}, err
	}
	var chosen accountpixiv.PoolCandidate
	found := false
	for _, candidate := range candidates {
		if candidate.UserID == chosenUserID {
			chosen = candidate
			found = true
			break
		}
	}
	if !found {
		return accountpixiv.Account{}, fmt.Errorf("database: pixiv pool chooser selected uid %d outside transaction snapshot", chosenUserID)
	}
	selected, err := scanPixivAccount(tx.QueryRowContext(ctx, `SELECT `+pixivAccountColumns+` FROM pixiv_account WHERE user_id=?`, chosen.UserID))
	if err != nil {
		return accountpixiv.Account{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET pool_last_selected = 0 WHERE pool_last_selected = 1`); err != nil {
		return accountpixiv.Account{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE pixiv_account SET pool_last_selected = 1, updated_at = ? WHERE user_id = ?`, now, chosen.UserID); err != nil {
		return accountpixiv.Account{}, err
	}
	if err := tx.Commit(); err != nil {
		return accountpixiv.Account{}, err
	}
	selected.PoolLastSelected = true
	return selected, nil
}

// FreezePooledPixiv 记录一个账号的有效冻结时间。
func (d *DB) FreezePooledPixiv(ctx context.Context, userID, frozenUntil int64) error {
	result, err := d.db.ExecContext(ctx, `UPDATE pixiv_account SET pool_frozen_until=CASE WHEN pool_frozen_until IS NULL OR pool_frozen_until < ? THEN ? ELSE pool_frozen_until END, updated_at=? WHERE user_id=?`,
		frozenUntil, frozenUntil, time.Now().UTC().Unix(), userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return accountpixiv.ErrNotFound
	}
	return nil
}

// SaveFanboxCredential 保存一次已验证的 FANBOX credential。
// 新账号固定从 revision 1 开始；已有账号在 SQL 内递增 revision。
func (d *DB) SaveFanboxCredential(ctx context.Context, account accountfanbox.Account) error {
	if account.UserID <= 0 || !account.HasSession() || account.ValidatedAt <= 0 {
		return errors.New("database: invalid fanbox account")
	}
	now := time.Now().UTC().Unix()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var sortOrder int64
	err = tx.QueryRowContext(ctx, `SELECT sort_order FROM fanbox_account WHERE user_id = ?`, account.UserID).Scan(&sortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		sortOrder = account.SortOrder
		if sortOrder <= 0 {
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) + 1 FROM fanbox_account`).Scan(&sortOrder); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO fanbox_account (`+fanboxAccountColumns+`) VALUES (?,?,?,?,?,?,?,?,?)`,
			account.UserID, sortOrder, account.DisplayName, account.CreatorID, account.SessionIDCopy(),
			1, account.ValidatedAt, now, now)
	} else if err != nil {
		return err
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE fanbox_account SET display_name=?, creator_id=?, session_id=?,
			credential_revision=credential_revision+1, validated_at=?, updated_at=? WHERE user_id=?`,
			account.DisplayName, account.CreatorID, account.SessionIDCopy(),
			account.ValidatedAt, now, account.UserID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RotateFanboxSession 更新 FANBOX session 并递增 credential_revision。
func (d *DB) RotateFanboxSession(ctx context.Context, userID, expectedRevision int64, session []byte, validatedAt int64) error {
	if userID <= 0 || expectedRevision <= 0 || len(session) == 0 || validatedAt <= 0 {
		return errors.New("database: invalid fanbox rotation input")
	}
	result, err := d.db.ExecContext(ctx,
		`UPDATE fanbox_account SET session_id=?, credential_revision=credential_revision+1, validated_at=?, updated_at=? WHERE user_id=? AND credential_revision=?`,
		session, validatedAt, time.Now().UTC().Unix(), userID, expectedRevision)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists int
		if err := d.db.QueryRowContext(ctx, `SELECT 1 FROM fanbox_account WHERE user_id=?`, userID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return accountfanbox.ErrNotFound
		} else if err != nil {
			return err
		}
		return accountfanbox.ErrCredentialConflict
	}
	return nil
}

// ListFanbox 返回全部 FANBOX 账号，按 sort_order 升序。
func (d *DB) ListFanbox(ctx context.Context) ([]accountfanbox.Account, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+fanboxAccountColumns+` FROM fanbox_account ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []accountfanbox.Account{}
	for rows.Next() {
		var userID, sortOrder, revision, validatedAt, createdAt, updatedAt int64
		var displayName, creatorID string
		var sessionID []byte
		if err := rows.Scan(&userID, &sortOrder, &displayName, &creatorID,
			&sessionID, &revision, &validatedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		out = append(out, newFanboxAccount(userID, sortOrder, displayName, creatorID, sessionID, revision, validatedAt, createdAt, updatedAt))
	}
	return out, rows.Err()
}

// GetFanbox 返回指定 user_id 的 FANBOX 账号；不存在返回 ErrNotFound。
func (d *DB) GetFanbox(ctx context.Context, userID int64) (accountfanbox.Account, error) {
	row := d.db.QueryRowContext(ctx, `SELECT `+fanboxAccountColumns+` FROM fanbox_account WHERE user_id = ?`, userID)
	account, err := scanFanboxAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return accountfanbox.Account{}, accountfanbox.ErrNotFound
	}
	return account, err
}

// RemoveFanbox 删除一个 FANBOX 账号。
func (d *DB) RemoveFanbox(ctx context.Context, userID int64) error {
	result, err := d.db.ExecContext(ctx, `DELETE FROM fanbox_account WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return accountfanbox.ErrNotFound
	}
	return nil
}

func scanPixivAccounts(rows *sql.Rows) ([]accountpixiv.Account, error) {
	out := []accountpixiv.Account{}
	for rows.Next() {
		account, err := scanPixivAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, account)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPixivAccount(scanner rowScanner) (accountpixiv.Account, error) {
	var userID, sortOrder, revision, createdAt, updatedAt int64
	var username string
	var refreshToken []byte
	var premiumCheckedAt, poolFrozenUntil *int64
	var premiumStatus *int64
	var poolLastSelected int
	var schedulable int
	err := scanner.Scan(&userID, &sortOrder, &username, &refreshToken, &revision,
		&premiumStatus, &premiumCheckedAt, &poolFrozenUntil, &poolLastSelected,
		&createdAt, &updatedAt, &schedulable)
	if err != nil {
		return accountpixiv.Account{}, err
	}
	account := accountpixiv.New(userID, username, refreshToken)
	account.SortOrder = sortOrder
	account.CredentialRevision = revision
	account.PremiumStatus = intPtrToBoolPtr(premiumStatus)
	account.PremiumCheckedAt = cloneInt64(premiumCheckedAt)
	account.PoolFrozenUntil = cloneInt64(poolFrozenUntil)
	account.PoolLastSelected = poolLastSelected == 1
	account.CreatedAt = createdAt
	account.UpdatedAt = updatedAt
	account.Schedulable = schedulable == 1
	return account, nil
}

func scanFanboxAccount(scanner rowScanner) (accountfanbox.Account, error) {
	var userID, sortOrder, revision, validatedAt, createdAt, updatedAt int64
	var displayName, creatorID string
	var sessionID []byte
	if err := scanner.Scan(&userID, &sortOrder, &displayName, &creatorID, &sessionID, &revision, &validatedAt, &createdAt, &updatedAt); err != nil {
		return accountfanbox.Account{}, err
	}
	return newFanboxAccount(userID, sortOrder, displayName, creatorID, sessionID, revision, validatedAt, createdAt, updatedAt), nil
}

func newFanboxAccount(userID, sortOrder int64, displayName, creatorID string, sessionID []byte, revision, validatedAt, createdAt, updatedAt int64) accountfanbox.Account {
	account := accountfanbox.New(userID, displayName, creatorID, sessionID)
	account.SortOrder = sortOrder
	account.CredentialRevision = revision
	account.ValidatedAt = validatedAt
	account.CreatedAt = createdAt
	account.UpdatedAt = updatedAt
	return account
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
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

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
