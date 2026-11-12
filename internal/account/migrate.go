package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
	_ "github.com/ncruces/go-sqlite3/driver"
)

const (
	legacyApplicationID     = 0x50495843
	legacyLatestVersion     = 3
	legacySchedulableColumn = "schedulable"
)

func importLegacy(ctx context.Context, path string) (document, error) {
	doc := emptyDocument()
	// 始终先持 JSON 锁，再持旧库锁；锁只保护快照，不执行任何 OAuth 请求。
	err := filelock.WithPrivateLock(ctx, path, func() (returnErr error) {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		escaped := (&url.URL{Path: filepath.ToSlash(path)}).EscapedPath()
		db, err := sql.Open("sqlite3", "file:"+escaped+"?mode=ro")
		if err != nil {
			return fmt.Errorf("account: open legacy database: %w", err)
		}
		defer func() { returnErr = errors.Join(returnErr, db.Close()) }()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return fmt.Errorf("account: read legacy snapshot: %w", err)
		}
		defer func() {
			if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
				returnErr = errors.Join(returnErr, err)
			}
		}()
		version, err := legacyVersion(ctx, tx)
		if err != nil {
			return err
		}
		if err := readLegacyPixiv(ctx, tx, version, &doc); err != nil {
			return err
		}
		if err := readLegacyFanbox(ctx, tx, &doc); err != nil {
			return err
		}
		return tx.Commit()
	})
	if err != nil {
		return document{}, fmt.Errorf("account: migrate legacy database: %w", err)
	}
	return doc, nil
}

func legacyVersion(ctx context.Context, tx *sql.Tx) (int, error) {
	var applicationID, version int
	if err := tx.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&applicationID); err != nil {
		return 0, err
	}
	if applicationID != legacyApplicationID {
		return 0, fmt.Errorf("legacy database belongs to another application (application_id=0x%X)", applicationID)
	}
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return 0, err
	}
	if version < 1 || version > legacyLatestVersion {
		return 0, fmt.Errorf("unsupported legacy database version %d", version)
	}
	rows, err := tx.QueryContext(ctx, `SELECT version FROM schema_migration ORDER BY version`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	applied := 0
	for rows.Next() {
		var next int
		if err := rows.Scan(&next); err != nil {
			return 0, err
		}
		applied++
		if next != applied {
			return 0, errors.New("legacy migration ledger has a version gap")
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if applied != version {
		return 0, errors.New("legacy migration ledger does not match user_version")
	}
	return version, nil
}

func readLegacyPixiv(ctx context.Context, tx *sql.Tx, version int, doc *document) error {
	// 前两个历史版本可能还没有 schedulable 列；当时所有账号都参与账号池。
	enabled := legacySchedulableColumn
	if version < legacyLatestVersion {
		var present int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('pixiv_account') WHERE name='schedulable'`).Scan(&present); err != nil {
			return err
		}
		if present == 0 {
			enabled = "1"
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT user_id,username,refresh_token,credential_revision,premium_status,premium_checked_at,pool_frozen_until,pool_last_selected,created_at,updated_at,`+enabled+` FROM pixiv_account ORDER BY sort_order`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a Pixiv
		var token []byte
		var premium, checked, frozen sql.NullInt64
		var selected bool
		var created, updated int64
		if err := rows.Scan(&a.UserID, &a.Username, &token, &a.CredentialRevision, &premium, &checked, &frozen, &selected, &created, &updated, &a.Pool.Enabled); err != nil {
			return err
		}
		a.Credential = credential.New(token)
		a.CreatedAt = time.Unix(created, 0).UTC()
		a.UpdatedAt = time.Unix(updated, 0).UTC()
		a.PremiumCheckedAt = legacyOptionalTime(checked)
		a.Pool.FrozenUntil = legacyOptionalTime(frozen)
		if premium.Valid {
			if premium.Int64 != 0 && premium.Int64 != 1 {
				return errors.New("invalid legacy premium status")
			}
			value := premium.Int64 == 1
			a.PremiumStatus = &value
		}
		if selected {
			if doc.LastUserID != 0 {
				return errors.New("multiple legacy pool markers")
			}
			doc.LastUserID = a.UserID
		}
		doc.Pixiv = append(doc.Pixiv, a)
	}
	return rows.Err()
}

func readLegacyFanbox(ctx context.Context, tx *sql.Tx, doc *document) error {
	rows, err := tx.QueryContext(ctx, `SELECT user_id,display_name,COALESCE(creator_id,''),session_id,credential_revision,validated_at,created_at,updated_at FROM fanbox_account ORDER BY sort_order`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a Fanbox
		var session []byte
		var validated, created, updated int64
		if err := rows.Scan(&a.UserID, &a.DisplayName, &a.CreatorID, &session, &a.CredentialRevision, &validated, &created, &updated); err != nil {
			return err
		}
		a.Credential = credential.New(session)
		a.ValidatedAt = time.Unix(validated, 0).UTC()
		a.CreatedAt = time.Unix(created, 0).UTC()
		a.UpdatedAt = time.Unix(updated, 0).UTC()
		doc.Fanbox = append(doc.Fanbox, a)
	}
	return rows.Err()
}

func legacyOptionalTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	result := time.Unix(value.Int64, 0).UTC()
	return &result
}
