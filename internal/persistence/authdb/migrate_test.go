package authdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAppliesMigrationsFromEmpty(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var userVersion int
	if err := db.db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if userVersion != CurrentVersion() {
		t.Fatalf("user_version = %d, want %d", userVersion, CurrentVersion())
	}
	var gotApplicationID int
	if err := db.db.QueryRow(`PRAGMA application_id`).Scan(&gotApplicationID); err != nil {
		t.Fatalf("application_id: %v", err)
	}
	if gotApplicationID != applicationID {
		t.Fatalf("application_id = 0x%X, want 0x%X", gotApplicationID, applicationID)
	}
	var count int
	if err := db.db.QueryRow(`SELECT count(*) FROM schema_migration`).Scan(&count); err != nil {
		t.Fatalf("schema_migration count: %v", err)
	}
	if count != len(embeddedMigrations) {
		t.Fatalf("schema_migration rows = %d, want %d", count, len(embeddedMigrations))
	}
}

func TestOpenRejectsWrongApplicationID(t *testing.T) {
	dir := t.TempDir()
	path := DatabasePath(dir)
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.db.Exec(`PRAGMA application_id = 0x1234`); err != nil {
		t.Fatalf("set application_id: %v", err)
	}
	_ = db.Close()
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "another application") {
		t.Fatalf("expected wrong application_id error, got %v", err)
	}
	_ = os.Remove(path)
}

func TestOpenRejectsDowngrade(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, CurrentVersion()+1)); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	_ = db.Close()
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("expected downgrade error, got %v", err)
	}
}

func TestMigrateFailsOnChecksumDrift(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE schema_migration SET checksum = 'drifted' WHERE version = 1`); err != nil {
		t.Fatalf("drift checksum: %v", err)
	}
	_ = db.Close()
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum drift error, got %v", err)
	}
}

func TestMigrateFailsOnNameDrift(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE schema_migration SET name = 'drifted' WHERE version = 1`); err != nil {
		t.Fatalf("drift name: %v", err)
	}
	_ = db.Close()
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "name drifted") {
		t.Fatalf("expected name drift error, got %v", err)
	}
}

func TestDatabasePermissions(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	info, err := os.Stat(db.Path())
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("db mode = %v, want 0600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, want 0700", dirInfo.Mode().Perm())
	}
}

func TestOpenIdempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	_ = db.Close()
	db2, err := Open(dir)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db2.Close()
}

func TestCurrentVersionIncludesAuthDBFollowupMigrations(t *testing.T) {
	if got := CurrentVersion(); got != 3 {
		t.Fatalf("CurrentVersion() = %d, want 3", got)
	}
}

func TestOpenMigratesV1CreatorAndSchedulableColumns(t *testing.T) {
	dir := t.TempDir()
	createV1Database(t, dir)

	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open migrated v1 database: %v", err)
	}
	defer db.Close()

	var creatorID string
	if err := db.db.QueryRow(`SELECT creator_id FROM fanbox_account WHERE user_id = 7`).Scan(&creatorID); err != nil {
		t.Fatalf("scan migrated creator_id: %v", err)
	}
	if creatorID != "" {
		t.Fatalf("creator_id = %q, want empty string", creatorID)
	}
	var schedulable int
	if err := db.db.QueryRow(`SELECT schedulable FROM pixiv_account WHERE user_id = 1`).Scan(&schedulable); err != nil {
		t.Fatalf("scan migrated schedulable: %v", err)
	}
	if schedulable != 1 {
		t.Fatalf("schedulable = %d, want 1", schedulable)
	}
	var checksumValue string
	if err := db.db.QueryRow(`SELECT checksum FROM schema_migration WHERE version = 1`).Scan(&checksumValue); err != nil {
		t.Fatalf("scan version 1 checksum: %v", err)
	}
	if checksumValue != embeddedMigrations[0].Checksum {
		t.Fatalf("version 1 checksum changed during follow-up migration")
	}
	var migrationCount int
	if err := db.db.QueryRow(`SELECT count(*) FROM schema_migration`).Scan(&migrationCount); err != nil {
		t.Fatalf("scan migration count: %v", err)
	}
	if migrationCount != 3 {
		t.Fatalf("migration count = %d, want 3", migrationCount)
	}
}

func TestPixivSchedulableCheckRejectsInvalidValue(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.db.Exec(`INSERT INTO pixiv_account (user_id, sort_order, username, refresh_token, credential_revision, pool_last_selected, created_at, updated_at, schedulable) VALUES (?,?,?,?,?,?,?,?,?)`, 1, 1, "user", []byte("token"), 1, 0, 1, 1, 2); err == nil {
		t.Fatal("invalid schedulable value was accepted")
	}
}

func TestOpenSupportsSpecialCharacterAndWindowsLikeDataDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "space unicode 名称 ? # %", `windows-like C:\pixiv\state`)
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open special data directory: %v", err)
	}
	defer db.Close()
	if err := db.SavePixivCredential(context.Background(), PixivAccount{UserID: 1, Username: "user", RefreshToken: []byte("token")}); err != nil {
		t.Fatalf("save special-path account: %v", err)
	}
	if _, err := db.GetPixiv(context.Background(), 1); err != nil {
		t.Fatalf("read special-path account: %v", err)
	}
}

func createV1Database(t *testing.T, appDataDir string) {
	t.Helper()
	if err := os.MkdirAll(appDataDir, 0o700); err != nil {
		t.Fatalf("create v1 app data directory: %v", err)
	}
	raw, err := sql.Open("sqlite", DatabasePath(appDataDir))
	if err != nil {
		t.Fatalf("open raw v1 database: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	if _, err := raw.Exec(fmt.Sprintf(`PRAGMA application_id = %d`, applicationID)); err != nil {
		t.Fatalf("set v1 application id: %v", err)
	}
	if _, err := raw.Exec(embeddedMigrations[0].SQL); err != nil {
		t.Fatalf("apply v1 schema: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO schema_migration (version, name, checksum, applied_at) VALUES (?,?,?,?)`, embeddedMigrations[0].Version, embeddedMigrations[0].Name, embeddedMigrations[0].Checksum, 1); err != nil {
		t.Fatalf("insert v1 migration ledger: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO pixiv_account (user_id, sort_order, username, refresh_token, credential_revision, pool_last_selected, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`, 1, 1, "user", []byte("token"), 1, 0, 1, 1); err != nil {
		t.Fatalf("insert v1 pixiv account: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO fanbox_account (user_id, sort_order, display_name, creator_id, session_id, credential_revision, validated_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, 7, 1, "creator", nil, []byte("session"), 1, 1, 1, 1); err != nil {
		t.Fatalf("insert v1 fanbox account: %v", err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("set v1 user_version: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw v1 database: %v", err)
	}
}
