// Package atomicfile 提供协议无关的原子落盘原语。
//
// 写入先在目标同目录创建临时文件并同步，再按平台替换目标；替换失败时由
// [Result] 说明目标与源文件的状态。
//
// 包的组成：[AtomicWrite] 流式写文件，[ReplaceFile] 与 [ReplaceFileWithBackup]
// 执行平台替换，[SyncParentDirectory] 同步目录项，[MustPreserveReplacementSource]
// 判断错误是否要求保留源文件。
package atomicfile

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// AtomicWrite 将 src 流式写入 path，并以同目录临时文件加 fsync/rename 完成原子提交。
func AtomicWrite(ctx context.Context, path string, src io.Reader) (written int64, resultErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	temp, err := os.CreateTemp(dir, ".atomic-write-*")
	if err != nil {
		return 0, err
	}
	tempPath := temp.Name()
	closed, preserveSource := false, false
	defer func() {
		if !closed {
			resultErr = errors.Join(resultErr, temp.Close())
		}
		if !preserveSource {
			if err := os.Remove(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				resultErr = errors.Join(resultErr, err)
			}
		}
	}()
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			count, err := temp.Write(buffer[:n])
			written += int64(count)
			if err != nil {
				return written, err
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				return written, readErr
			}
			break
		}
	}
	if err := temp.Sync(); err != nil {
		return written, err
	}
	closed = true
	if err := temp.Close(); err != nil {
		return written, err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return written, err
	}
	if err := ctx.Err(); err != nil {
		return written, err
	}
	replacement, err := ReplacePrivateFile(tempPath, path)
	preserveSource = replacement.PreserveSource
	return written, err
}
