package mediaproxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"io"
	"os"
	"time"
)

// PublishAnimation retains already validated, complete encoded bytes for protected
// download from the current account-owner read snapshot. It never fetches metadata
// or encodes; failure leaves inline delivery intact.
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
	// 编码跨越凭据轮换时，旧快照的产物不能取得新版本的授权。
	if source == nil || lease.Lease.Value() != source {
		return out, errCapability
	}
	c.Quality = format
	digest := sha256.New()
	digest.Write(data)
	digest.Write(preview)
	key := contentKey(c, lease, "animation-inline") + ":" + hex.EncodeToString(digest.Sum(nil))
	a := s.animations
	a.mu.Lock()
	if a.closed || a.ctx.Err() != nil || s.authorized(ctx, c) != nil {
		a.mu.Unlock()
		return out, errCapability
	}
	if existing := a.jobs[key]; existing != nil {
		if existing.state == "ready" && time.Now().Before(existing.expiry) {
			existing.last = time.Now()
			out = s.animationOutputLocked(existing, "")
			a.mu.Unlock()
			return out, nil
		}
		// 正在发布的同键副本不能替换，避免使正在读取的文件失去 owner。
		a.mu.Unlock()
		return out, errors.New("animation download not ready")
	}
	id, err := animationHandle()
	if err != nil {
		a.mu.Unlock()
		return out, err
	}
	dir, err := os.MkdirTemp(a.root, "job-")
	if err != nil {
		a.mu.Unlock()
		return out, err
	}
	workerCtx, cancel := context.WithCancel(a.ctx)
	j := &animationJob{key: key, id: id, version: lease.Version, format: format, dir: dir, claim: c, expiry: s.expiry(c, lease), state: "encoding", ctx: workerCtx, cancel: cancel, last: time.Now(), size: int64(len(data))}
	// 已知完整长度，写入前一次预留，避免不同填充者互相耗尽剩余空间。
	if int64(len(preview)) > a.capacity || int64(len(data)) > a.capacity-int64(len(preview)) {
		err = errAnimationCapacity
	} else {
		err = a.reserveLocked(j, int64(len(data))+int64(len(preview)))
	}
	if err != nil {
		j.done = true
		j.state = "failed"
		cancel()
		a.cleanLocked(j)
		a.mu.Unlock()
		return out, err
	}
	a.jobs[key] = j
	a.wg.Add(1)
	if lease.Invalidated != nil {
		a.sessionWG.Add(1)
		go func() {
			defer a.sessionWG.Done()
			select {
			case <-workerCtx.Done():
			case <-lease.Invalidated:
				a.mu.Lock()
				defer a.mu.Unlock()
				if workerCtx.Err() == nil {
					a.invalidateLocked(j, "expired", "authorization")
				}
			}
		}()
	}
	a.mu.Unlock()
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
	}{{"animation." + format, data}, {"preview.png", preview}} {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if err = workerCtx.Err(); err != nil {
			return out, err
		}
		file, openErr := animationFile(dir, item.name)
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
