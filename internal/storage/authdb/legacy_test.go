package authdb

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const legacyFixture = `{
  "default_user_id": 456,
  "accounts": [
    {"refresh_token": "token-one", "user_id": 456, "username": "first", "premium_status": true},
    {"refresh_token": "token-two", "user_id": 789, "username": "second"}
  ]
}`

func TestMigrateLegacyAuthJSON(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(legacyPath, []byte(legacyFixture), 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	result, err := MigrateLegacyAuthJSON(ctx, dir, legacyPath)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if result.Skipped || !result.Imported || result.AccountCount != 2 || result.DefaultUserID != 456 {
		t.Fatalf("result = %+v", result)
	}
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("open after migrate: %v", err)
	}
	defer db.Close()
	accounts, err := db.ListPixiv(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(accounts) != 2 || string(accounts[0].RefreshToken) != "token-one" || accounts[0].SortOrder != 1 {
		t.Fatalf("accounts = %+v", accounts)
	}
}

func TestMigrateLegacyAuthJSONSkippedWhenAbsent(t *testing.T) {
	result, err := MigrateLegacyAuthJSON(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !result.Skipped {
		t.Fatalf("expected skipped, got %+v", result)
	}
}

func TestMigrateLegacyAuthJSONReentrantConsistent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(legacyPath, []byte(legacyFixture), 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if _, err := MigrateLegacyAuthJSON(ctx, dir, legacyPath); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// 数据库已导入但旧 JSON 仍存在：逻辑一致则继续成功。
	result, err := MigrateLegacyAuthJSON(ctx, dir, legacyPath)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if result.Imported {
		t.Fatal("second migrate should not re-import")
	}
}

func TestMigrateLegacyAuthJSONReentrantInconsistent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(legacyPath, []byte(legacyFixture), 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if _, err := MigrateLegacyAuthJSON(ctx, dir, legacyPath); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// 篡改数据库账号造成与 legacy 不一致。
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := db.RotatePixivCredentials(ctx, 456, []byte("tampered")); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := MigrateLegacyAuthJSON(ctx, dir, legacyPath); err == nil || !strings.Contains(err.Error(), "comparison failed") {
		t.Fatalf("expected comparison failure, got %v", err)
	}
}

func TestMigrateLegacyAuthJSONIncompleteAccountFails(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "auth.json")
	fixture := `{"accounts":[{"user_id":0,"refresh_token":""}]}`
	if err := os.WriteFile(legacyPath, []byte(fixture), 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if _, err := MigrateLegacyAuthJSON(context.Background(), dir, legacyPath); err == nil {
		t.Fatal("expected incomplete account failure")
	}
}
