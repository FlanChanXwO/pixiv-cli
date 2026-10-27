// Package filelock 用同目录侧车文件为本地状态的读改写事务提供跨进程互斥。
//
// 锁文件位于目标路径旁，不随目标文件的原子替换而消失。
//
// 包的组成：[WithPrivateLock] 持有锁执行一个动作，[TryPrivateLock] 只尝试获取一次。
package filelock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
)

// WithPrivateLock 在原子替换目标文件之外使用同目录侧车锁串行化事务。锁文件不随
// JSON replacement 被替换，因此多个 CLI/MCP 进程始终竞争同一个协调点。
func WithPrivateLock(ctx context.Context, path string, action func() error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := openLockFile(path)
	if err != nil {
		return err
	}
	release, err := acquireContext(ctx, file)
	if err != nil {
		return errors.Join(err, file.Close())
	}
	defer func() { err = errors.Join(err, release()) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return action()
}

func acquireContext(ctx context.Context, file *os.File) (func() error, error) {
	if ctx.Done() == nil {
		return acquire(file)
	}
	release, acquired, err := tryAcquire(file)
	if acquired || err != nil {
		return release, err
	}
	// 原生阻塞锁不能被context唤醒；仅争用时轮询锁，不重放事务、不设总等待期限。
	// 10ms限制空转开销；取消直接由Done唤醒，不等待下次检查。
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			release, acquired, err := tryAcquire(file)
			if acquired || err != nil {
				return release, err
			}
		}
	}
}

// TryPrivateLock 尝试获取一次实例锁，不等待正在持锁的 owner。
func TryPrivateLock(path string) (release func() error, acquired bool, err error) {
	file, err := openLockFile(path)
	if err != nil {
		return nil, false, err
	}
	release, acquired, err = tryAcquire(file)
	if !acquired {
		err = errors.Join(err, file.Close())
	}
	return
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
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}
