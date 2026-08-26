//go:build unix

package reversesearch_test

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/stretchr/testify/require"
)

func TestFileSourceRejectsFIFOWithoutOpeningAReader(t *testing.T) {
	source := filepath.Join(t.TempDir(), "image.fifo")
	require.NoError(t, syscall.Mkfifo(source, 0o600))
	loader := reversesearch.NewSourceLoader(reversesearch.SourceLoaderOptions{TempDir: t.TempDir()})

	_, err := loader.Load(context.Background(), source)
	require.Equal(t, reversesearch.CodeSourceNotRegularFile, reversesearch.CodeOf(err))
	require.EqualError(t, err, "image source must be a regular file")
}
