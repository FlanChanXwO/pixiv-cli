package mediaproxy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
)

const animationWorkspaceMarker = "pixiv-mcp-animation-v1\n"

func newAnimationWorkspace(parent string) (string, func() error, error) {
	if err := cleanupAnimationWorkspaces(parent); err != nil {
		return "", nil, err
	}
	root, err := os.MkdirTemp(parent, "pixiv-mcp-animation-")
	if err != nil {
		return "", nil, err
	}
	release, acquired, err := filelock.TryPrivateLock(filepath.Join(root, "instance"))
	if err != nil || !acquired {
		_ = os.RemoveAll(root)
		return "", nil, errors.Join(err, errors.New("animation instance lock unavailable"))
	}
	// 先取得生命周期文件锁，再发布所有权标记。
	if err = os.WriteFile(filepath.Join(root, "owner"), []byte(animationWorkspaceMarker), 0600); err != nil {
		_ = release()
		_ = os.RemoveAll(root)
		return "", nil, err
	}
	return root, release, nil
}

func cleanupAnimationWorkspaces(parent string) error {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "pixiv-mcp-animation-") {
			continue
		}
		root := filepath.Join(parent, entry.Name())
		marker := filepath.Join(root, "owner")
		info, err := os.Lstat(marker)
		if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(animationWorkspaceMarker)) {
			continue
		}
		body, err := os.ReadFile(marker)
		if err != nil || string(body) != animationWorkspaceMarker {
			continue
		}
		// 能取得 OS 文件锁说明当前没有进程持有该目录，不必依赖可能被复用的 PID。
		release, acquired, err := filelock.TryPrivateLock(filepath.Join(root, "instance"))
		if err != nil {
			return err
		}
		if !acquired {
			continue
		}
		if err = os.Remove(marker); err != nil && !os.IsNotExist(err) {
			_ = release()
			return err
		}
		if err = release(); err != nil {
			return err
		}
		if err = os.RemoveAll(root); err != nil {
			return err
		}
	}
	return nil
}
