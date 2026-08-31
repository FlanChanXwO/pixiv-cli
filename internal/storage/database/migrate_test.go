package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
)

func TestOpenAppliesMigrationsAndApplicationID(t *testing.T) {
	if CurrentVersion() != 3 {
		t.Fatalf("CurrentVersion() = %d, want 3", CurrentVersion())
	}
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var userVersion, applicationID, migrationCount int
	if err := db.DB().QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatal(err)
	}
	if userVersion != CurrentVersion() {
		t.Fatalf("user_version = %d, want %d", userVersion, CurrentVersion())
	}
	if err := db.DB().QueryRow(`PRAGMA application_id`).Scan(&applicationID); err != nil {
		t.Fatal(err)
	}
	if applicationID != ApplicationID {
		t.Fatalf("application_id = 0x%X, want 0x%X", applicationID, ApplicationID)
	}
	if err := db.DB().QueryRow(`SELECT count(*) FROM schema_migration`).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount != 3 {
		t.Fatalf("schema_migration count = %d, want 3", migrationCount)
	}
}

func TestOpenAcceptsLegacyInitialMigrationChecksum(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE schema_migration SET checksum=? WHERE version=1`, "0247f7ea8739433ce47074048a1c8707728e7f3d04cf47c3ff6f15282f8e641f"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	legacyDB, err := Open(dir)
	if err != nil {
		t.Fatalf("legacy initial migration checksum was rejected: %v", err)
	}
	defer legacyDB.Close()
}

func TestOpenIsIdempotentAndRejectsApplicationOrVersionDrift(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`PRAGMA application_id = 0x1234`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "another application") {
		t.Fatalf("wrong application_id error = %v", err)
	}
	_ = os.Remove(DatabasePath(dir))

	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(fmt.Sprintf(`PRAGMA user_version = %d`, CurrentVersion()+1)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "downgrade") {
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
			db, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.DB().Exec(test.query); err != nil {
				t.Fatal(err)
			}
			db.Close()
			if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("migration drift error = %v", err)
			}
		})
	}
}

func TestDatabasePermissionsAndSpecialPath(t *testing.T) {
	specialDirectory := "space unicode 名称 ? # %"
	nestedDirectory := `windows-like C:\pixiv\state`
	if runtime.GOOS == "windows" {
		// ?、: 与反斜杠在 Windows 路径中具有特殊含义，不能作为普通目录名。
		specialDirectory = "space unicode 名称 # %"
		nestedDirectory = "windows-like C pixiv state"
	}
	dir := filepath.Join(t.TempDir(), specialDirectory, nestedDirectory)
	db, err := Open(dir)
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
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("database mode = %v, err=%v", info.Mode().Perm(), err)
	}
	dirInfo, err := os.Stat(filepath.Dir(db.Path()))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, err=%v", dirInfo.Mode().Perm(), err)
	}
}

func TestInitialSchemaUsesFinalV1Defaults(t *testing.T) {
	db := openTestDatabase(t)
	if _, err := db.DB().Exec(`INSERT INTO pixiv_account (user_id, sort_order, username, refresh_token, credential_revision, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`, 1, 1, "user", []byte("token"), 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	var schedulable int
	if err := db.DB().QueryRow(`SELECT schedulable FROM pixiv_account WHERE user_id=1`).Scan(&schedulable); err != nil {
		t.Fatal(err)
	}
	if schedulable != 1 {
		t.Fatalf("schedulable = %d, want 1", schedulable)
	}
	if _, err := db.DB().Exec(`INSERT INTO fanbox_account (user_id, sort_order, display_name, session_id, credential_revision, validated_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`, 7, 1, "creator", []byte("session"), 1, 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	var creatorID string
	if err := db.DB().QueryRow(`SELECT creator_id FROM fanbox_account WHERE user_id=7`).Scan(&creatorID); err != nil {
		t.Fatal(err)
	}
	if creatorID != "" {
		t.Fatalf("creator_id = %q, want empty", creatorID)
	}
}

func TestPixivSchedulableCheckRejectsInvalidValue(t *testing.T) {
	db := openTestDatabase(t)
	if _, err := db.DB().Exec(`INSERT INTO pixiv_account (user_id, sort_order, username, refresh_token, credential_revision, pool_last_selected, created_at, updated_at, schedulable) VALUES (?,?,?,?,?,?,?,?,?)`, 1, 1, "user", []byte("token"), 1, 0, 1, 1, 2); err == nil {
		t.Fatal("invalid schedulable value was accepted")
	}
}

func openTestDatabase(t *testing.T) *DB {
	t.Helper()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestTableInfoQueryUsesFixedIdentifiers(t *testing.T) {
	for _, test := range []struct {
		table string
		want  string
	}{
		{table: "fanbox_account", want: `PRAGMA table_info(fanbox_account)`},
		{table: "pixiv_account", want: `PRAGMA table_info(pixiv_account)`},
	} {
		t.Run(test.table, func(t *testing.T) {
			query, err := tableInfoQuery(test.table)
			if err != nil {
				t.Fatalf("tableInfoQuery(%q): %v", test.table, err)
			}
			if query != test.want {
				t.Fatalf("tableInfoQuery(%q) = %q, want %q", test.table, query, test.want)
			}
		})
	}

	if _, err := tableInfoQuery("pixiv_account); DROP TABLE schema_migration;--"); err == nil {
		t.Fatal("tableInfoQuery accepted an unsupported identifier")
	}
}
