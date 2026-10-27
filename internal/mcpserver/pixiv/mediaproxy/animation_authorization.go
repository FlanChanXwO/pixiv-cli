package mediaproxy

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/fsnotify/fsnotify"
)

func (s *Service) startAnimationAuthorizationWatch() error {
	path, err := filepath.Abs(s.store.Path)
	if err != nil {
		return err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	// 授权状态用原子替换发布，所以监听父目录；旧文件的 inode 在替换后失效。
	dir := filepath.Dir(path)
	if err := watcher.Add(dir); err != nil {
		return errors.Join(err, watcher.Close())
	}
	// Windows 只报告目录内部的变化，父目录监听补上目录本身被移动的通知。
	if runtime.GOOS == "windows" {
		if err := watcher.Add(filepath.Dir(dir)); err != nil {
			return errors.Join(err, watcher.Close())
		}
	}
	a := s.animations
	a.watchDone = make(chan struct{})
	go func() {
		defer close(a.watchDone)
		defer func() {
			if err := watcher.Close(); err != nil {
				a.mu.Lock()
				a.watchErr = errors.Join(a.watchErr, err)
				a.mu.Unlock()
			}
		}()
		for {
			select {
			case <-a.ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok || (event.Name == dir && event.Has(fsnotify.Remove|fsnotify.Rename)) {
					s.animationAuthorizationWatchFailed()
					return
				}
				if filepath.Clean(event.Name) == path {
					s.revalidateAnimationGrants()
				}
			case _, ok := <-watcher.Errors:
				if a.ctx.Err() == nil || !ok {
					s.animationAuthorizationWatchFailed()
				}
				return
			}
		}
	}()
	return nil
}

func (s *Service) revalidateAnimationGrants() {
	a := s.animations
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.ctx.Err() != nil {
		return
	}
	// 读取与枚举共用发布锁，稍后加入的任务只接受新快照。
	state, err := s.store.Read(a.ctx)
	for _, job := range a.jobs {
		grant, ok := state.Grants[job.claim.Grant]
		if err != nil || !ok || !grant.Usable(s.base+"/mcp") || !time.Now().Before(job.claim.Expires) {
			a.invalidateLocked(job, "expired", "authorization")
		}
	}
	s.purgeStaticGrants()
}

func (s *Service) purgeStaticGrants() {
	if s.reads == nil {
		return
	}
	var state auth.State
	var err error
	read := false
	s.reads.cache.invalidate(func(key string) bool {
		if !read {
			state, err = s.store.Read(s.animations.ctx)
			read = true
		}
		parts := strings.SplitN(key, "/", 3)
		if err != nil || len(parts) < 3 {
			return true
		}
		grant, ok := state.Grants[parts[1]]
		return !ok || !grant.Usable(s.base+"/mcp")
	})
}

func (s *Service) animationAuthorizationWatchFailed() {
	a := s.animations
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ctx.Err() != nil {
		return
	}
	// 无法观察撤销时停止接收与交付；关闭时返回固定错误，不暴露私密路径。
	a.watchErr = errors.New("animation authorization watcher failed")
	a.closed = true
	for _, job := range a.jobs {
		a.invalidateLocked(job, "expired", "authorization")
	}
	a.cancel()
	s.purgeStaticGrants()
}
