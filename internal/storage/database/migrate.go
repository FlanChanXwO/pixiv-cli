// Package database 管理 Pixiv / FANBOX 鉴权状态的单一 SQLite 数据库。
// 它只保存鉴权数据与 Pixiv account-pool 租约状态，不保存普通配置、下载
// archive、缓存、请求日志或浏览器 profile。
package database

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// applicationID 防止误开其他 SQLite 数据库。值取 ASCII "PIXC"。
const applicationID = ApplicationID

// migration 是嵌入脚本的一条记录；checksum 是该脚本完整内容的 SHA-256。
type migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

// embeddedMigrations 是嵌入的所有迁移，按版本升序。
var embeddedMigrations []migration

func init() {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		panic("database: cannot read embedded migrations: " + err.Error())
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		content, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			panic("database: cannot read embedded migration " + name + ": " + err.Error())
		}
		embeddedMigrations = append(embeddedMigrations, migration{
			Version:  migrationVersion(name),
			Name:     migrationName(name),
			SQL:      string(content),
			Checksum: checksum(content),
		})
	}
	sort.Slice(embeddedMigrations, func(i, j int) bool {
		return embeddedMigrations[i].Version < embeddedMigrations[j].Version
	})
}

func migrationVersion(filename string) int {
	underscore := strings.IndexByte(filename, '_')
	if underscore <= 0 {
		panic("database: malformed migration filename " + filename)
	}
	version, err := strconv.Atoi(filename[:underscore])
	if err != nil || version <= 0 {
		panic("database: malformed migration version in " + filename)
	}
	return version
}

func migrationName(filename string) string {
	return strings.TrimSuffix(filename, ".sql")
}

func checksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// CurrentVersion 返回 binary 所知的最新 schema 版本。
func CurrentVersion() int {
	if len(embeddedMigrations) == 0 {
		return 0
	}
	return embeddedMigrations[len(embeddedMigrations)-1].Version
}

type appliedMigration struct {
	Version  int
	Name     string
	Checksum string
}

// Released pre-v1 binaries recorded this initial-schema checksum before the
// current tree consolidated the later columns into 0001; keep that known
// ledger value valid so existing local databases can advance to v3.
const legacyInitialMigrationChecksum = "0247f7ea8739433ce47074048a1c8707728e7f3d04cf47c3ff6f15282f8e641f"

// migrate 将数据库 schema 推进到 binary 的当前版本。它只向前执行：checksum
// 漂移、版本缺口、重复版本、未知更新 schema 和 downgrade 一律 fail closed。
func migrate(db *sql.DB) error {
	var userVersion int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	// 全新数据库尚无 schema_migration；按是否存在该表决定是否读取已应用记录，
	// 避免应用 0001 前的崩溃残留被误判。
	var hasMigrationTable int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_migration'`).Scan(&hasMigrationTable); err != nil {
		return fmt.Errorf("inspect schema_migration: %w", err)
	}
	applied := map[int]appliedMigration{}
	if hasMigrationTable != 0 {
		read, err := readApplied(db)
		if err != nil {
			return err
		}
		applied = read
		if err := validateApplied(applied); err != nil {
			return err
		}
	}
	// 拒绝 schema 比 binary 更新的 downgrade 运行。
	latest := CurrentVersion()
	if userVersion > latest {
		return fmt.Errorf("database schema version %d is newer than binary schema version %d; refusing to run a downgrade", userVersion, latest)
	}
	for _, m := range embeddedMigrations {
		if existing, ok := applied[m.Version]; ok {
			if existing.Name != m.Name {
				return fmt.Errorf("migration %d name drifted: %q != %q", m.Version, existing.Name, m.Name)
			}
			if existing.Checksum != m.Checksum && !(m.Version == 1 && existing.Checksum == legacyInitialMigrationChecksum) {
				return fmt.Errorf("migration %d checksum drifted", m.Version)
			}
			continue
		}
		alreadySatisfied, err := migrationAlreadySatisfied(db, m)
		if err != nil {
			return fmt.Errorf("inspect migration %d (%s): %w", m.Version, m.Name, err)
		}
		if alreadySatisfied {
			// 0001 in this tree already contains the final v3 columns. Record the
			// historical migrations without replaying duplicate ALTER TABLE SQL.
			if err := recordOne(db, m); err != nil {
				return fmt.Errorf("record migration %d (%s): %w", m.Version, m.Name, err)
			}
			continue
		}
		if err := applyOne(db, m); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Name, err)
		}
	}
	if len(embeddedMigrations) > 0 {
		if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, latest)); err != nil {
			return fmt.Errorf("set user_version: %w", err)
		}
	}
	return nil
}

func readApplied(db *sql.DB) (map[int]appliedMigration, error) {
	rows, err := db.Query(`SELECT version, name, checksum FROM schema_migration ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migration: %w", err)
	}
	defer rows.Close()
	out := map[int]appliedMigration{}
	for rows.Next() {
		var a appliedMigration
		if err := rows.Scan(&a.Version, &a.Name, &a.Checksum); err != nil {
			return nil, err
		}
		out[a.Version] = a
	}
	return out, rows.Err()
}

func validateApplied(applied map[int]appliedMigration) error {
	if len(applied) == 0 {
		return nil
	}
	versions := make([]int, 0, len(applied))
	for version := range applied {
		versions = append(versions, version)
	}
	sort.Ints(versions)
	for i, version := range versions {
		if i == 0 {
			if version != 1 {
				return fmt.Errorf("migration version gap: first applied version is %d, want 1", version)
			}
			continue
		}
		if version != versions[i-1]+1 {
			return fmt.Errorf("migration version gap: %d follows %d", version, versions[i-1])
		}
	}
	return nil
}

func applyOne(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(m.SQL); err != nil {
		return err
	}
	if err := recordOneTx(tx, m); err != nil {
		return err
	}
	return tx.Commit()
}

func recordOne(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := recordOneTx(tx, m); err != nil {
		return err
	}
	return tx.Commit()
}

func recordOneTx(tx *sql.Tx, m migration) error {
	if _, err := tx.Exec(
		`INSERT INTO schema_migration (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		m.Version, m.Name, m.Checksum, time.Now().UTC().Unix(),
	); err != nil {
		return err
	}
	return nil
}

func migrationAlreadySatisfied(db *sql.DB, m migration) (bool, error) {
	switch m.Version {
	case 2:
		return tableColumnNotNull(db, "fanbox_account", "creator_id")
	case 3:
		present, _, err := tableColumn(db, "pixiv_account", "schedulable")
		return present, err
	default:
		return false, nil
	}
}

func tableColumnNotNull(db *sql.DB, table, column string) (bool, error) {
	present, notNull, err := tableColumn(db, table, column)
	return present && notNull, err
}

func tableColumn(db *sql.DB, table, column string) (present, notNull bool, returnErr error) {
	if table != "fanbox_account" && table != "pixiv_account" {
		return false, false, fmt.Errorf("unsupported table %q", table)
	}
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, false, err
	}
	defer func() {
		if closeErr := rows.Close(); returnErr == nil && closeErr != nil {
			returnErr = closeErr
		}
	}()
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultV   sql.NullString
			primary    int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultV, &primary); err != nil {
			return false, false, err
		}
		if name == column {
			return true, notNull != 0, rows.Err()
		}
	}
	return false, false, rows.Err()
}
