package mediaproxy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/file/lock"
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
	release, acquired, err := lock.TryPrivateLock(filepath.Join(root, "instance"))
	if err != nil || !acquired {
		_ = os.RemoveAll(root)
		return "", nil, errors.Join(err, errors.New("animation instance lock unavailable"))
	}
	// Publish the ownership marker only after taking the lifetime lock.
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
		// A released OS lock proves termination without trusting a reused PID.
		release, acquired, err := lock.TryPrivateLock(filepath.Join(root, "instance"))
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
