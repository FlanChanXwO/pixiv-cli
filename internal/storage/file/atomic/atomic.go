// Package atomicfile 提供协议无关的原子落盘原语。
package atomic

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// Write 将 src 流式写入 Path 的原子目标：先写入目标目录中的私有临时文件并
// fsync，再 rename 到最终路径，因此部分写入的文件绝不会出现在最终路径。任何
// 失败都会移除临时文件。
func Write(ctx context.Context, path string, src io.Reader) (int64, error) {
	return AtomicWrite(ctx, path, src)
}

// AtomicWrite 将 src 流式写入 path，并以同目录临时文件加 fsync/rename 完成原子提交。
// Write 保留为已有调用方的兼容名称。
func AtomicWrite(ctx context.Context, path string, src io.Reader) (int64, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	temp, err := os.CreateTemp(dir, ".atomic-write-*")
	if err != nil {
		return 0, err
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	var written int64
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			_ = temp.Close()
			return written, err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			if _, err := temp.Write(buffer[:n]); err != nil {
				_ = temp.Close()
				return written, err
			}
			written += int64(n)
		}
		if readErr != nil {
			if readErr != io.EOF {
				_ = temp.Close()
				return written, readErr
			}
			break
		}
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return written, err
	}
	if err := temp.Close(); err != nil {
		return written, err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return written, err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return written, err
	}
	return written, nil
}
