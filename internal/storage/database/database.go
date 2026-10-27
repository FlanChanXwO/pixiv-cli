// Package database 管理 Pixiv 与 FANBOX 鉴权状态的单一 SQLite 数据库。
//
// 数据库只保存鉴权数据与 Pixiv 账号池调度状态，不保存普通配置、下载 archive、缓存、
// 请求日志或浏览器 profile。
//
// 包的组成：[Open] 打开或创建数据库并完成迁移，[DB] 提供账号读写端口；
// repository.go 与 pool.go 分别承载账号查询和账号池调度，migrate.go 维护迁移 ledger。
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/gormlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// DB 持有鉴权数据库的 SQL 与 GORM 句柄；调用方使用后必须 Close。
type DB struct {
	sql  *sql.DB
	orm  *gorm.DB
	path string
}

// DatabasePath 返回鉴权数据库在本机私有目录下的固定路径。
func DatabasePath(appDataDir string) string {
	return filepath.Join(appDataDir, "pixiv-cli.db")
}

// Open 打开或创建 appDataDir 下的鉴权数据库，收紧目录与文件权限，设置连接
// pragma，并应用全部 schema migration。调用方必须 Close。
func Open(appDataDir string) (*DB, error) {
	if strings.TrimSpace(appDataDir) == "" {
		return nil, fmt.Errorf("database: app data directory is required")
	}
	if err := os.MkdirAll(appDataDir, 0o700); err != nil {
		return nil, fmt.Errorf("database: create app data directory: %w", err)
	}
	path := DatabasePath(appDataDir)
	dsn := dsnWithPragmas(path)
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("database: open database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	orm, err := gorm.Open(gormlite.OpenDB(sqlDB), &gorm.Config{
		Logger:                 logger.Discard,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		NamingStrategy:         schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("database: open orm: %w", err)
	}
	db := &DB{sql: sqlDB, orm: orm, path: path}
	// 首次连接和迁移也参与协调，避免另一进程的监听读取撞上schema写入。
	if err := db.withLock(context.Background(), func() error {
		if err := sqlDB.Ping(); err != nil {
			return fmt.Errorf("database: ping database: %w", err)
		}
		return db.initialize()
	}); err != nil {
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
	query.Add("_pragma", "busy_timeout(0)")
	// ncruces 把 URI 直接交给 SQLite；Windows 应写成 file:C:/...，不能再加 "///"。
	// 路径仍需按 URI 转义，含空格、?#% 的目录才能被正确解析。
	escaped := (&url.URL{Path: filepath.ToSlash(path)}).EscapedPath()
	return "file:" + escaped + "?" + query.Encode()
}

// 同一路径的CLI/MCP短事务和完整快照读取共用侧车锁；不持锁执行上游请求。
func (d *DB) withLock(ctx context.Context, action func() error) error {
	return filelock.WithPrivateLock(ctx, d.path, action)
}

func withDatabaseValue[T any](ctx context.Context, d *DB, action func() (T, error)) (T, error) {
	var value T
	err := d.withLock(ctx, func() error {
		var err error
		value, err = action()
		return err
	})
	return value, err
}

func (d *DB) initialize() error {
	if err := d.enforcePermissions(); err != nil {
		return err
	}
	if err := d.verifyOrSetApplicationID(); err != nil {
		return err
	}
	return migrate(d.sql)
}

// enforcePermissions 在 Unix-like 平台收紧目录为 0700、数据库为 0600。Windows
// 上由上层负责私有 ACL，此处不声称 POSIX mode。
func (d *DB) enforcePermissions() error {
	dir := filepath.Dir(d.path)
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("database: tighten directory permissions: %w", err)
	}
	if err := os.Chmod(d.path, 0o600); err != nil {
		return fmt.Errorf("database: tighten database permissions: %w", err)
	}
	return nil
}

func (d *DB) verifyOrSetApplicationID() error {
	var id int
	if err := d.sql.QueryRow(`PRAGMA application_id`).Scan(&id); err != nil {
		return fmt.Errorf("database: read application_id: %w", err)
	}
	if id == 0 {
		// 全新数据库或尚无归属标记的数据库：设置固定 application_id。
		if _, err := d.sql.Exec(fmt.Sprintf(`PRAGMA application_id = %d`, applicationID)); err != nil {
			return fmt.Errorf("database: set application_id: %w", err)
		}
		return nil
	}
	if id != applicationID {
		return fmt.Errorf("database: database belongs to another application (application_id=0x%X)", id)
	}
	return nil
}

// Close 关闭底层数据库。
func (d *DB) Close() error {
	return d.sql.Close()
}

// Path 返回数据库文件路径。
func (d *DB) Path() string { return d.path }
