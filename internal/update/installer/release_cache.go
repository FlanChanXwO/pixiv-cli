package installer

import (
	"context"
	"fmt"
	"os"

	filereplace "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/replace"
	"github.com/FlanChanXwO/pixiv-cli/internal/update/release"
	"github.com/FlanChanXwO/pixiv-cli/internal/update/source"
)

// NewFileReleaseCache 建立 release 包的存储端口：以私密权限原子写入 cache 文件。
// cachePath 是包含固定文件名的完整目标路径。
func NewFileReleaseCache(cacheDir, cachePath string) release.ReleaseCache {
	return &fileReleaseCache{
		cacheDir:    cacheDir,
		cachePath:   cachePath,
		replaceFile: filereplace.ReplaceFile,
	}
}

// fileReleaseCache 实现 release.ReleaseCache，把 GitHub Releases cache 保存在
// 0700 目录中的 0600 文件里，并在同一目录原子替换。
type fileReleaseCache struct {
	cacheDir  string
	cachePath string
	// replaceFile 是每个 cache 独立的替换 seam；生产构造固定使用 filereplace.ReplaceFile。
	replaceFile func(string, string) error
}

func (c *fileReleaseCache) Read(ctx context.Context) ([]byte, bool, error) {
	if err := source.CheckContext(ctx, "read GitHub Releases cache"); err != nil {
		return nil, false, err
	}
	cacheBytes, err := os.ReadFile(c.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read GitHub Releases cache %q: %w", c.cachePath, err)
	}
	return cacheBytes, true, nil
}

func (c *fileReleaseCache) Write(ctx context.Context, body []byte) (err error) {
	if err := source.CheckContext(ctx, "write GitHub Releases cache"); err != nil {
		return err
	}
	if err := ensurePrivateReleaseCacheDirectory(c.cacheDir); err != nil {
		return err
	}
	cacheFile, err := os.CreateTemp(c.cacheDir, ".github-releases-*")
	if err != nil {
		return fmt.Errorf("create temporary GitHub Releases cache in %q: %w", c.cacheDir, err)
	}
	temporaryPath := cacheFile.Name()
	defer func() {
		_ = cacheFile.Close()
		if !filereplace.MustPreserveReplacementSource(err) {
			if removeErr := os.Remove(temporaryPath); err == nil && removeErr != nil && !os.IsNotExist(removeErr) {
				err = fmt.Errorf("remove temporary GitHub Releases cache %q: %w", temporaryPath, removeErr)
			}
		}
	}()
	if err := cacheFile.Chmod(0o600); err != nil {
		return fmt.Errorf("set temporary GitHub Releases cache permissions %q: %w", temporaryPath, err)
	}
	if _, err := cacheFile.Write(body); err != nil {
		return fmt.Errorf("encode GitHub Releases cache %q: %w", temporaryPath, err)
	}
	if err := cacheFile.Sync(); err != nil {
		return fmt.Errorf("sync temporary GitHub Releases cache %q: %w", temporaryPath, err)
	}
	if err := cacheFile.Close(); err != nil {
		return fmt.Errorf("close temporary GitHub Releases cache %q: %w", temporaryPath, err)
	}
	// ReplaceFile 在 Windows 对已有目标使用 ReplaceFileW，首次创建才用不覆盖的 MoveFileEx。
	if err := c.replaceFile(temporaryPath, c.cachePath); err != nil {
		return fmt.Errorf("atomically replace GitHub Releases cache %q: %w", c.cachePath, err)
	}
	return nil
}
