package database_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/account/pixiv"
	database "github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
)

func TestOpenAppliesMigrationsAndApplicationID(t *testing.T) {
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var userVersion, applicationID, migrationCount int
	if err := db.DB().QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatal(err)
	}
	if userVersion != database.CurrentVersion() {
		t.Fatalf("user_version = %d, want %d", userVersion, database.CurrentVersion())
	}
	if err := db.DB().QueryRow(`PRAGMA application_id`).Scan(&applicationID); err != nil {
		t.Fatal(err)
	}
	if applicationID != database.ApplicationID {
		t.Fatalf("application_id = 0x%X, want 0x%X", applicationID, database.ApplicationID)
	}
	if err := db.DB().QueryRow(`SELECT count(*) FROM schema_migration`).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount != database.CurrentVersion() {
		t.Fatalf("schema_migration count = %d, want %d", migrationCount, database.CurrentVersion())
	}
}

func TestOpenIsIdempotentAndRejectsApplicationOrVersionDrift(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`PRAGMA application_id = 0x1234`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := database.Open(dir); err == nil || !strings.Contains(err.Error(), "another application") {
		t.Fatalf("wrong application_id error = %v", err)
	}
	_ = os.Remove(database.DatabasePath(dir))

	db, err = database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(fmt.Sprintf(`PRAGMA user_version = %d`, database.CurrentVersion()+1)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := database.Open(dir); err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("downgrade error = %v", err)
	}
}

func TestMigrationLedgerRejectsChecksumAndNameDrift(t *testing.T) {
	for _, test := range []struct {
		name  string
		query string
		want  string
	}{
		{name: "checksum", query: `UPDATE schema_migration SET checksum='drifted' WHERE version=1`, want: "checksum"},
		{name: "name", query: `UPDATE schema_migration SET name='drifted' WHERE version=1`, want: "name drifted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			db, err := database.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.DB().Exec(test.query); err != nil {
				t.Fatal(err)
			}
			db.Close()
			if _, err := database.Open(dir); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("migration drift error = %v", err)
			}
		})
	}
}

func TestDatabasePermissionsAndSpecialPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "space unicode 名称 ? # %", `windows-like C:\pixiv\state`)
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SavePixivCredential(context.Background(), accountpixiv.New(1, "user", []byte("token"))); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetPixiv(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(db.Path())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("database mode = %v, err=%v", info.Mode().Perm(), err)
	}
	dirInfo, err := os.Stat(filepath.Dir(db.Path()))
	if err != nil || dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, err=%v", dirInfo.Mode().Perm(), err)
	}
}

func TestOpenMigratesV1WithoutChangingOriginalChecksum(t *testing.T) {
	dir := t.TempDir()
	createV1Database(t, dir)
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var creatorID string
	if err := db.DB().QueryRow(`SELECT creator_id FROM fanbox_account WHERE user_id=7`).Scan(&creatorID); err != nil {
		t.Fatal(err)
	}
	if creatorID != "" {
		t.Fatalf("creator_id = %q, want empty", creatorID)
	}
	var schedulable int
	if err := db.DB().QueryRow(`SELECT schedulable FROM pixiv_account WHERE user_id=1`).Scan(&schedulable); err != nil {
		t.Fatal(err)
	}
	if schedulable != 1 {
		t.Fatalf("schedulable = %d, want 1", schedulable)
	}
	var checksum string
	if err := db.DB().QueryRow(`SELECT checksum FROM schema_migration WHERE version=1`).Scan(&checksum); err != nil {
		t.Fatal(err)
	}
	if checksum != migrationChecksum(t, "0001_initial.sql") {
		t.Fatalf("version 1 checksum changed: %s", checksum)
	}
	var count int
	if err := db.DB().QueryRow(`SELECT count(*) FROM schema_migration`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("migration count = %d, err=%v", count, err)
	}
}

func TestPixivSchedulableCheckRejectsInvalidValue(t *testing.T) {
	db := openTestDatabase(t)
	if _, err := db.DB().Exec(`INSERT INTO pixiv_account (user_id, sort_order, username, refresh_token, credential_revision, pool_last_selected, created_at, updated_at, schedulable) VALUES (?,?,?,?,?,?,?,?,?)`, 1, 1, "user", []byte("token"), 1, 0, 1, 1, 2); err == nil {
		t.Fatal("invalid schedulable value was accepted")
	}
}

func openTestDatabase(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func migrationBytes(t *testing.T, filename string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("migrations", filename))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func migrationChecksum(t *testing.T, filename string) string {
	t.Helper()
	sum := sha256.Sum256(migrationBytes(t, filename))
	return hex.EncodeToString(sum[:])
}

func createV1Database(t *testing.T, appDataDir string) {
	t.Helper()
	if err := os.MkdirAll(appDataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", database.DatabasePath(appDataDir))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	body := migrationBytes(t, "0001_initial.sql")
	if _, err := raw.Exec(fmt.Sprintf(`PRAGMA application_id = %d`, database.ApplicationID)); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO schema_migration (version, name, checksum, applied_at) VALUES (1,?,?,?)`, "0001_initial", migrationChecksum(t, "0001_initial.sql"), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO pixiv_account (user_id, sort_order, username, refresh_token, credential_revision, pool_last_selected, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`, 1, 1, "user", []byte("token"), 1, 0, 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO fanbox_account (user_id, sort_order, display_name, creator_id, session_id, credential_revision, validated_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`, 7, 1, "creator", nil, []byte("session"), 1, 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
}
