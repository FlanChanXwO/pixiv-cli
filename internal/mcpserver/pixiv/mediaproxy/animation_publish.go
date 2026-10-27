package mediaproxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// PublishAnimation 把已经校验、完整编码的字节保存为当前账号读快照下的受保护下载；
// 它不重新拉取元数据也不编码。发布失败不影响原有的内联交付。
func (s *Service) PublishAnimation(ctx context.Context, account, artwork int64, format string, source *pixiv.Client, data, preview []byte) (out AnimationStatus, err error) {
	if s == nil || s.animations == nil || artwork <= 0 || (format != "gif" && format != "apng") || len(data) == 0 || len(preview) == 0 {
		return out, errCapability
	}
	c, err := s.animationClaims(ctx, account, artwork)
	if err != nil {
		return out, err
	}
	lease, err := s.session(ctx, account)
	if err != nil {
		return out, err
	}
	defer lease.Lease.Close()
	// 编码期间凭据可能已轮换，旧快照的产物只按旧版本授权。
	if source == nil || lease.Lease.Value() != source {
		return out, errCapability
	}
	c.Quality = format
	key := contentKey(c, lease, "animation-inline") + ":" + animationDigest(data, preview)
	j, ready, err := s.reserveAnimation(ctx, lease, c, format, key, int64(len(data))+int64(len(preview)))
	if err != nil {
		return out, err
	}
	if ready != nil {
		return *ready, nil
	}
	return s.writeAnimation(ctx, j, data, preview)
}

func animationDigest(data, preview []byte) string {
	digest := sha256.New()
	digest.Write(data)
	digest.Write(preview)
	return hex.EncodeToString(digest.Sum(nil))
}

// reserveAnimation 在锁内复核授权、复用已就绪的副本并预留完整下载字节。ready 非 nil
// 时调用方直接返回它，不再启动写入。
func (s *Service) reserveAnimation(ctx context.Context, lease Session, c claims, format, key string, size int64) (j *animationJob, ready *AnimationStatus, err error) {
	a := s.animations
	a.mu.Lock()
	if a.closed || a.ctx.Err() != nil || s.authorized(ctx, c) != nil {
		a.mu.Unlock()
		return nil, nil, errCapability
	}
	if existing := a.jobs[key]; existing != nil {
		defer a.mu.Unlock()
		if existing.state == "ready" && time.Now().Before(existing.expiry) {
			existing.last = time.Now()
			out := s.animationOutputLocked(existing, "")
			return nil, &out, nil
		}
		// 同键副本正在发布时保留原文件，正在读取的文件继续有 owner。
		return nil, nil, errors.New("animation download not ready")
	}
	id, err := animationHandle()
	if err != nil {
		a.mu.Unlock()
		return nil, nil, err
	}
	dir, err := os.MkdirTemp(a.root, "job-")
	if err != nil {
		a.mu.Unlock()
		return nil, nil, err
	}
	workerCtx, cancel := context.WithCancel(a.ctx)
	j = &animationJob{key: key, id: id, version: lease.Version, format: format, dir: dir, claim: c, expiry: s.expiry(c, lease), state: "encoding", ctx: workerCtx, cancel: cancel, last: time.Now(), size: size}
	// 长度已知，所以写入前一次预留，避免多个写入者把剩余空间各自耗光。
	err = a.reserveLocked(j, size)
	if err != nil {
		j.done = true
		j.state = "failed"
		cancel()
		a.cleanLocked(j)
		a.mu.Unlock()
		return nil, nil, err
	}
	a.jobs[key] = j
	a.wg.Add(1)
	a.watchSessionLocked(j, lease.Invalidated)
	a.mu.Unlock()
	return j, nil, nil
}

// writeAnimation 写入已验证的字节并发布下载 capability；失败时撤销预留并删除工作目录。
func (s *Service) writeAnimation(ctx context.Context, j *animationJob, data, preview []byte) (out AnimationStatus, err error) {
	a := s.animations
	// 获取会话时也会持有 a.mu；这里只在最终发布阶段持锁。
	defer a.wg.Done()
	defer func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		j.done = true
		if err != nil || j.ctx.Err() != nil || !time.Now().Before(j.expiry) {
			if err == nil {
				err = errCapability
			}
			a.invalidateLocked(j, "expired", "authorization")
			out = AnimationStatus{}
			return
		}
		j.state = "ready"
		j.timer = time.AfterFunc(time.Until(j.expiry), func() { a.mu.Lock(); defer a.mu.Unlock(); a.invalidateLocked(j, "expired", "expiry") })
		out = s.animationOutputLocked(j, "")
	}()
	for _, item := range []struct {
		name string
		data []byte
	}{{"animation." + j.format, data}, {"preview.png", preview}} {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if err = j.ctx.Err(); err != nil {
			return out, err
		}
		file, openErr := animationFile(j.dir, item.name)
		if openErr != nil {
			return out, openErr
		}
		n, writeErr := file.Write(item.data)
		closeErr := file.Close()
		if n != len(item.data) && writeErr == nil {
			writeErr = io.ErrShortWrite
		}
		if err = errors.Join(writeErr, closeErr); err != nil {
			return out, err
		}
	}
	err = s.animationSessionValid(ctx, j)
	return out, err
}
