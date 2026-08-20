package lock

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
)

// WithPrivateLock 在原子替换目标文件之外使用同目录侧车锁串行化事务。锁文件不随
// JSON replacement 被替换，因此多个 CLI/MCP 进程始终竞争同一个协调点。
func WithPrivateLock(ctx context.Context, path string, action func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if action == nil {
		return errors.New("local state action is not configured")
	}
	file, err := openLockFile(path)
	if err != nil {
		return err
	}
	release, err := acquire(file)
	if err != nil {
		_ = file.Close()
		return err
	}
	defer func() { _ = release() }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return action()
}

func openLockFile(path string) (*os.File, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, paths.PrivateDirMode); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, paths.PrivateDirMode); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, paths.PrivateFileMode)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(paths.PrivateFileMode); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
