// Private GitHub release cache persistence using the shared atomic writer.

package installer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate/release"
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate/source"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
)

// NewFileReleaseCache stores release metadata at a private, atomically replaced path.
func NewFileReleaseCache(cachePath string) release.ReleaseCache {
	return &fileReleaseCache{cachePath: cachePath}
}

type fileReleaseCache struct{ cachePath string }

func (c *fileReleaseCache) Read(ctx context.Context) ([]byte, bool, error) {
	if err := source.CheckContext(ctx, "read GitHub Releases cache"); err != nil {
		return nil, false, err
	}
	body, err := os.ReadFile(c.cachePath)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read GitHub Releases cache %q: %w", c.cachePath, err)
	}
	return body, true, nil
}

func (c *fileReleaseCache) Write(ctx context.Context, body []byte) error {
	if err := source.CheckContext(ctx, "write GitHub Releases cache"); err != nil {
		return err
	}
	if err := ensurePrivateReleaseCacheDirectory(filepath.Dir(c.cachePath)); err != nil {
		return err
	}
	if _, err := atomicfile.AtomicWrite(ctx, c.cachePath, bytes.NewReader(body)); err != nil {
		return fmt.Errorf("atomically write GitHub Releases cache %q: %w", c.cachePath, err)
	}
	return nil
}
