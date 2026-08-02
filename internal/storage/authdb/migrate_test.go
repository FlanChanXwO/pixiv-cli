package authdb

import (
	"fmt"
	"os"
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
	var applicationID int
	if err := db.db.QueryRow(`PRAGMA application_id`).Scan(&applicationID); err != nil {
		t.Fatalf("application_id: %v", err)
	}
	if applicationID != applicationID {
		t.Fatalf("application_id = 0x%X", applicationID)
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
