package authdb

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// DatabasePath 返回鉴权数据库在本机私有目录下的固定路径。
func DatabasePath(appDataDir string) string {
	return filepath.Join(appDataDir, "pixiv-cli.db")
}

// Open 打开或创建 appDataDir 下的鉴权数据库，收紧目录与文件权限，设置连接
// pragma，并应用全部 schema migration。调用方必须 Close。
func Open(appDataDir string) (*DB, error) {
	if strings.TrimSpace(appDataDir) == "" {
		return nil, fmt.Errorf("authdb: app data directory is required")
	}
	if err := os.MkdirAll(appDataDir, 0o700); err != nil {
		return nil, fmt.Errorf("authdb: create app data directory: %w", err)
	}
	path := DatabasePath(appDataDir)
	dsn := dsnWithPragmas(path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("authdb: open database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	// sql.Open 不建立连接；先 ping 以暴露路径/权限错误。
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("authdb: ping database: %w", err)
	}
	db := &DB{db: sqlDB, path: path}
	if err := db.initialize(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func dsnWithPragmas(path string) string {
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "synchronous(FULL)")
	query.Add("_pragma", "secure_delete(ON)")
	query.Add("_pragma", "trusted_schema(off)")
	return (&url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}).String()
}

type DB struct {
	db         *sql.DB
	path       string
	poolRandom func(int) (int, error)
}

func (d *DB) initialize() error {
	if err := d.enforcePermissions(); err != nil {
		return err
	}
	if err := d.verifyOrSetApplicationID(); err != nil {
		return err
	}
	if err := migrate(d.db); err != nil {
		return err
	}
	return nil
}

// enforcePermissions 在 Unix-like 平台收紧目录为 0700、数据库为 0600。Windows
// 上由上层负责私有 ACL，此处不声称 POSIX mode。
func (d *DB) enforcePermissions() error {
	dir := filepath.Dir(d.path)
	if err := os.Chmod(dir, 0o700); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("authdb: tighten directory permissions: %w", err)
	}
	if _, err := os.Stat(d.path); err == nil {
		if err := os.Chmod(d.path, 0o600); err != nil {
			return fmt.Errorf("authdb: tighten database permissions: %w", err)
		}
	}
	return nil
}

func (d *DB) verifyOrSetApplicationID() error {
	var id int
	if err := d.db.QueryRow(`PRAGMA application_id`).Scan(&id); err != nil {
		return fmt.Errorf("authdb: read application_id: %w", err)
	}
	if id == 0 {
		// 全新数据库或尚无归属标记的数据库：设置固定 application_id。
		if _, err := d.db.Exec(fmt.Sprintf(`PRAGMA application_id = %d`, applicationID)); err != nil {
			return fmt.Errorf("authdb: set application_id: %w", err)
		}
		return nil
	}
	if id != applicationID {
		return fmt.Errorf("authdb: database belongs to another application (application_id=0x%X)", id)
	}
	return nil
}

// Close 关闭底层数据库。
func (d *DB) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

// Path 返回数据库文件路径。
func (d *DB) Path() string { return d.path }

// DB 返回底层 *sql.DB，供 repository 与测试使用。
func (d *DB) DB() *sql.DB { return d.db }
