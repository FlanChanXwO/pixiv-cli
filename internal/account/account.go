// Package account 管理 Pixiv 与 FANBOX 的本地账号、凭据和账号池状态。
//
// [Open] 在文件锁内读取或迁移 auth.json；[Store] 的读取每次取得当前文件快照，
// 不持有 SQLite 连接。默认账号与网络配置由调用方管理。
//
// 包的组成：[Store] 保存 [Pixiv] 与 [Fanbox] 的有序快照和凭据版本，
// [ReadSessions] 按版本复用读取客户端，[Scheduler] 编排安全的账号池重放，
// [LoginService] 处理一次性登录与凭据保存。
package account

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
)

const (
	authFilename   = "auth.json"
	legacyFilename = "pixiv-cli.db"
	authSchema     = "pixiv-cli.auth"
	authVersion    = 1
)

// Store 只保存账号文件的位置；每次操作在侧车锁内重新读取，跨进程共享最新状态。
type Store struct{ path string }

// Open 打开 directory 下的账号文件；首次运行迁移旧库或创建空文件。
// 迁移失败保留原文件，旧库始终只读。调用方传入私有应用目录，不读取用户 HOME。
func Open(ctx context.Context, directory string) (*Store, error) {
	s := &Store{path: filepath.Join(directory, authFilename)}
	err := s.withLock(ctx, func() error { _, err := s.read(ctx); return err })
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) withLock(ctx context.Context, action func() error) error {
	return filelock.WithPrivateLock(ctx, s.path, action)
}

func (s *Store) read(ctx context.Context) (document, error) {
	body, err := secretfile.ReadPrivateFile(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return document{}, fmt.Errorf("account: read file: %w", err)
	}
	doc, migrate, err := decodeDocument(body, errors.Is(err, os.ErrNotExist))
	if err != nil {
		return document{}, err
	}
	if !migrate {
		return doc, nil
	}
	doc, err = importLegacy(ctx, filepath.Join(filepath.Dir(s.path), legacyFilename))
	if err != nil {
		return document{}, err
	}
	if err := ctx.Err(); err != nil {
		return document{}, err
	}
	if err = s.write(doc); err != nil {
		return document{}, err
	}
	return doc, nil
}

// ListPixiv 返回文件顺序的独立账号快照；每次调用都会观察其他进程已提交的修改。
func (s *Store) ListPixiv(ctx context.Context) ([]Pixiv, error) {
	var result []Pixiv
	err := s.withLock(ctx, func() error {
		doc, err := s.read(ctx)
		result = doc.Pixiv
		for i := range result {
			result[i].PoolLastSelected = result[i].UserID == doc.LastUserID
		}
		return err
	})
	return result, err
}

// ListFanbox 返回文件顺序的独立 FANBOX 账号快照。
func (s *Store) ListFanbox(ctx context.Context) ([]Fanbox, error) {
	var result []Fanbox
	err := s.withLock(ctx, func() error { doc, err := s.read(ctx); result = doc.Fanbox; return err })
	return result, err
}
