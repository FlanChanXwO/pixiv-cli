//go:build unix

// source_unix_test.go 使用 internal test package（package reversesearch），因为本文件
// 需要覆盖未导出的 Loader.openFile 字段来模拟 TOCTOU 竞态——open 与 stat 之间文件类型
// 被替换为目录。这是 reversesearch 包内唯一的 same-package 测试例外：source_test.go 和
// source_fifo_test.go 均使用 external test package（reversesearch_test）。

package reversesearch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileSourceRechecksTypeAfterOpen(t *testing.T) {
	source := filepath.Join(t.TempDir(), "image")
	require.NoError(t, os.WriteFile(source, []byte("original"), 0o600))
	snapshotDir := t.TempDir()
	loader := NewSourceLoader(SourceLoaderOptions{TempDir: snapshotDir})
	loader.openFile = func(path string) (*os.File, error) {
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.Mkdir(path, 0o700))
		return os.Open(path)
	}

	_, err := loader.Load(context.Background(), source)
	require.Equal(t, CodeSourceNotRegularFile, CodeOf(err))
	require.EqualError(t, err, "image source must be a regular file")
	entries, readErr := os.ReadDir(snapshotDir)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}
