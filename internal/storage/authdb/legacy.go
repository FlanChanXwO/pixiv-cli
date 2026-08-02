package authdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// legacyAuthStore 匹配旧 auth.json 的稳定字段子集，只读取迁移所需内容。
type legacyAuthStore struct {
	DefaultUserID int64           `json:"default_user_id,omitempty"`
	Accounts      []legacyAccount `json:"accounts,omitempty"`
}

type legacyAccount struct {
	RefreshToken           string     `json:"refresh_token"`
	UserID                 int64      `json:"user_id,omitempty"`
	Username               string     `json:"username,omitempty"`
	PremiumStatus          *bool      `json:"premium_status,omitempty"`
	PremiumStatusCheckedAt *time.Time `json:"premium_status_checked_at,omitempty"`
}

// LegacyMigrationResult 报告 auth.json → SQLite 迁移的结果。
type LegacyMigrationResult struct {
	// Skipped 表示没有 legacy store 需要迁移。
	Skipped bool
	// Imported 表示本次执行完成了导入。
	Imported bool
	// AccountCount 是迁移后数据库中的账号数。
	AccountCount int
	// DefaultUserID 是旧 store 的默认账号，调用方负责写入 [pixiv.auth]。
	DefaultUserID int64
}

// MigrateLegacyAuthJSON 将 legacy auth.json 一次性迁移到数据库。迁移可安全重入：
// 如果数据库已导入但旧 JSON 仍存在，先完整逻辑对比；一致则继续，不一致 fail
// closed。迁移成功后调用方应写入 config 并删除旧文件；旧文件删除失败时数据库
// 已提交，legacy secret 仍存在，由调用方明确报告。
func MigrateLegacyAuthJSON(ctx context.Context, appDataDir, legacyPath string) (LegacyMigrationResult, error) {
	data, err := os.ReadFile(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return LegacyMigrationResult{Skipped: true}, nil
	}
	if err != nil {
		return LegacyMigrationResult{}, fmt.Errorf("authdb: read legacy auth store: %w", err)
	}
	var legacy legacyAuthStore
	if err := json.Unmarshal(data, &legacy); err != nil {
		return LegacyMigrationResult{}, fmt.Errorf("authdb: decode legacy auth store: %w", err)
	}
	db, err := Open(appDataDir)
	if err != nil {
		return LegacyMigrationResult{}, err
	}
	defer db.Close()

	existing, err := db.ListPixiv(ctx)
	if err != nil {
		return LegacyMigrationResult{}, err
	}
	imported := len(existing) == 0
	if imported {
		for index, account := range legacy.Accounts {
			if account.UserID <= 0 || account.RefreshToken == "" {
				return LegacyMigrationResult{}, fmt.Errorf("authdb: legacy account %d is incomplete", index)
			}
			var premiumCheckedAt *int64
			if account.PremiumStatusCheckedAt != nil {
				value := account.PremiumStatusCheckedAt.UTC().Unix()
				premiumCheckedAt = &value
			}
			err := db.UpsertPixiv(ctx, PixivAccount{
				UserID:             account.UserID,
				SortOrder:          int64(index + 1),
				Username:           account.Username,
				RefreshToken:       []byte(account.RefreshToken),
				CredentialRevision: 1,
				PremiumStatus:      account.PremiumStatus,
				PremiumCheckedAt:   premiumCheckedAt,
			})
			if err != nil {
				return LegacyMigrationResult{}, fmt.Errorf("authdb: import legacy account %d: %w", index, err)
			}
		}
		if err := verifyLegacyComparison(ctx, db, legacy); err != nil {
			return LegacyMigrationResult{}, err
		}
	} else {
		if err := verifyLegacyComparison(ctx, db, legacy); err != nil {
			return LegacyMigrationResult{}, err
		}
	}
	accounts, err := db.ListPixiv(ctx)
	if err != nil {
		return LegacyMigrationResult{}, err
	}
	return LegacyMigrationResult{
		Imported:      imported,
		AccountCount:  len(accounts),
		DefaultUserID: legacy.DefaultUserID,
	}, nil
}

// verifyLegacyComparison 逐账号比较数据库与 legacy store，保证导入可验证且重入
// 不会丢失数据。
func verifyLegacyComparison(ctx context.Context, db *DB, legacy legacyAuthStore) error {
	accounts, err := db.ListPixiv(ctx)
	if err != nil {
		return err
	}
	if len(accounts) != len(legacy.Accounts) {
		return fmt.Errorf("authdb: legacy comparison failed: database has %d accounts, legacy has %d", len(accounts), len(legacy.Accounts))
	}
	for index, legacyAccount := range legacy.Accounts {
		got := accounts[index]
		if got.UserID != legacyAccount.UserID || got.Username != legacyAccount.Username ||
			string(got.RefreshToken) != legacyAccount.RefreshToken {
			return fmt.Errorf("authdb: legacy comparison failed at account %d", index)
		}
	}
	return nil
}

// RemoveLegacyAuthJSON 原子删除迁移后的旧 auth.json。删除失败时调用方必须明确
// 报告数据库已提交且 legacy secret 仍存在。
func RemoveLegacyAuthJSON(path string) error {
	return os.Remove(path)
}
