// Package vectorindex 保存 Pixiv 作品快照、待合并记录和按模型划分的页面向量。
//
// [Recorder] 只追加私有记录，不打开索引或联网；[Open] 打开独立的 SQLite 索引，
// [Store.Merge] 恢复并合并记录，[Store.Sync] 续读收藏并逐页计算向量。
// 配置与账号选择由调用方负责。
package vectorindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
	_ "github.com/ncruces/go-sqlite3/driver"
)

const (
	directoryName = "vector"
	indexName     = "index.db"
	inboxName     = "inbox.ndjson"
	mergingName   = "inbox.merging"
	rejectedName  = "inbox.rejected.ndjson"
	syncLockName  = "vector-sync"
	inboxLockName = "vector-inbox"
	applicationID = 0x50585649
	schemaVersion = 1
)

// Store 持有向量索引；写入方须先取得 [TrySyncLock]，读取方可并行打开。
type Store struct {
	db        *sql.DB
	directory string
}

// Directory 返回应用目录内的向量状态路径。
func Directory(root string) string { return filepath.Join(root, directoryName) }

// TrySyncLock 尝试获取跨进程同步锁；release 的错误也须交给调用方。
func TrySyncLock(root string) (release func() error, acquired bool, err error) {
	return filelock.TryPrivateLock(filepath.Join(root, syncLockName))
}

// Open 创建或打开私有索引，拒绝其他应用的数据和未知版本。
func Open(ctx context.Context, root string) (*Store, error) {
	directory := Directory(root)
	if err := secretfile.EnsurePrivateDir(directory); err != nil {
		return nil, err
	}
	path := filepath.Join(directory, indexName)
	if err := secretfile.EnsurePrivateFile(path, nil, config.PrivateFileMode); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, config.PrivateFileMode); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, directory: directory}
	if err = s.initialize(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

func (s *Store) initialize(ctx context.Context) error {
	var id, version, tables int
	if err := s.db.QueryRowContext(ctx, `PRAGMA application_id`).Scan(&id); err != nil {
		return err
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if id == 0 && version == 0 {
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			return errors.New("vector index belongs to another application")
		}
		_, err := s.db.ExecContext(ctx, fmt.Sprintf(`BEGIN IMMEDIATE;
   CREATE TABLE artwork(id TEXT PRIMARY KEY, record BLOB NOT NULL, recorded_at TEXT NOT NULL);
   CREATE TABLE page(artwork_id TEXT NOT NULL REFERENCES artwork(id) ON DELETE CASCADE, page_index INTEGER NOT NULL, url TEXT NOT NULL, unavailable TEXT NOT NULL DEFAULT '', PRIMARY KEY(artwork_id,page_index));
   CREATE TABLE embedding(artwork_id TEXT NOT NULL, page_index INTEGER NOT NULL, model TEXT NOT NULL, vector BLOB, failure TEXT NOT NULL DEFAULT '', PRIMARY KEY(artwork_id,page_index,model), FOREIGN KEY(artwork_id,page_index) REFERENCES page(artwork_id,page_index) ON DELETE CASCADE);
   CREATE TABLE bookmark(account_id INTEGER NOT NULL, restrict TEXT NOT NULL, artwork_id TEXT NOT NULL, PRIMARY KEY(account_id,restrict,artwork_id));
   CREATE TABLE state(key TEXT PRIMARY KEY, value TEXT NOT NULL);
   PRAGMA application_id=%d; PRAGMA user_version=%d; COMMIT;`, applicationID, schemaVersion))
		if err != nil {
			return err
		}
	} else if id != applicationID || version != schemaVersion {
		return fmt.Errorf("unsupported vector index (application=%d, version=%d)", id, version)
	}
	_, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
	return err
}

// Close 释放索引连接。
func (s *Store) Close() error { return s.db.Close() }
