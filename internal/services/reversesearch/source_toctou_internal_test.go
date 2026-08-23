//go:build unix

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
