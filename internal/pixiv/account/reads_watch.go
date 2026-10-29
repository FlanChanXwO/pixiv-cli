package account

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/fsnotify/fsnotify"
)

// Watch 把外部凭据提交绑定到读取会话生命周期。只有 HTTP MCP 组合根会启动该 watcher；
// stdio 与普通 CLI 调用不会。
func (m *ReadSessions) Watch(ctx context.Context, path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.watchDone != nil {
		return errors.New("read session watcher cannot be started")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := watcher.Add(dir); err != nil {
		return errors.Join(err, watcher.Close())
	}
	// Windows 只报告目录内部变化；父目录监听补齐目录本身被移动的通知。
	if runtime.GOOS == "windows" {
		if err := watcher.Add(filepath.Dir(dir)); err != nil {
			return errors.Join(err, watcher.Close())
		}
	}
	watchCtx, cancel := context.WithCancel(ctx)
	m.watchCancel = cancel
	m.watchDone = make(chan struct{})
	go func() {
		defer close(m.watchDone)
		defer func() {
			closeErr := watcher.Close()
			m.mu.Lock()
			defer m.mu.Unlock()
			m.watchErr = errors.Join(m.watchErr, closeErr)
			m.closed = true
			for uid, entry := range m.sessions {
				entry.close()
				delete(m.sessions, uid)
			}
		}()
		fail := func() {
			m.mu.Lock()
			if watchCtx.Err() == nil && m.ctx.Err() == nil {
				m.watchErr = errors.New("read session credential watcher failed")
			}
			m.mu.Unlock()
		}
		for {
			select {
			case <-watchCtx.Done():
				return
			case event, ok := <-watcher.Events:
				name := filepath.Clean(event.Name)
				// kqueue可合并目录Write/Rename并移除watch；不能只依赖单独的Rename通知。
				if !ok || !slices.Contains(watcher.WatchList(), dir) || ((name == dir || name == path) && event.Has(fsnotify.Remove|fsnotify.Rename)) {
					fail()
					return
				}
				// 查询由数据库owner与写事务协调；-shm只协调读者，避免自身读事件循环。
				changed := name == path || name == path+"-wal" || name == path+"-journal"
				if changed {
					if err := m.Revalidate(watchCtx); err != nil && watchCtx.Err() == nil {
						fail()
						return
					}
				}
			case watchErr, ok := <-watcher.Errors:
				var pathErr *os.PathError
				if ok && errors.As(watchErr, &pathErr) && errors.Is(watchErr, os.ErrNotExist) && pathErr.Op == "lstat" && filepath.Clean(pathErr.Path) == path+"-journal" && slices.Contains(watcher.WatchList(), dir) {
					// kqueue识别新journal时它可能已提交删除；主库和watch仍在则读取提交后的版本。
					if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
						if err := m.Revalidate(watchCtx); err == nil {
							continue
						}
					}
				}
				if watchCtx.Err() == nil {
					fail()
				}
				return
			}
		}
	}()
	return nil
}
