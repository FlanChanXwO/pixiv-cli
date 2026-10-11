package mediaproxy

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
)

var errAnimationCapacity = errors.New("animation temporary capacity exhausted")

type animationState struct {
	ctx              context.Context
	cancel           context.CancelFunc
	root             string
	releaseWorkspace func() error
	capacity, used   int64
	mu               sync.Mutex
	wg               sync.WaitGroup
	sessionWG        sync.WaitGroup
	buildGate        chan struct{}
	jobs             map[string]*animationJob
	handles          map[string]*animationJob
	closed           bool
	watchDone        chan struct{}
	watchErr         error
	cleanupErr       error
	cacheSessions    map[string]struct{}
}
type animationJob struct {
	key, id, version, format, dir string
	claim                         claims
	expiry                        time.Time
	state, code                   string
	cancellationSource            string
	deliveryCancellationSource    string
	ctx                           context.Context
	cancel                        context.CancelFunc
	done                          bool
	subscribers, pins             int
	used                          int64
	size                          int64
	last                          time.Time
	timer                         *time.Timer
}

// ConfigureAnimations creates this service's private workspace before any requests.
// Unknown legacy directories are deliberately left alone: their ownership cannot
// be proved by a directory name or a reused process ID.
func (s *Service) ConfigureAnimations(ctx context.Context, capacity int64) error {
	if capacity <= 0 || s.reads == nil || s.animations != nil {
		return errors.New("invalid animation configuration")
	}
	root, release, err := newAnimationWorkspace(os.TempDir())
	if err != nil {
		return err
	}
	if err = os.Chmod(root, 0700); err != nil {
		_ = release()
		_ = os.RemoveAll(root)
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	s.animations = &animationState{ctx: ctx, cancel: cancel, root: root, releaseWorkspace: release, capacity: capacity, buildGate: make(chan struct{}, 1), jobs: map[string]*animationJob{}, handles: map[string]*animationJob{}, cacheSessions: map[string]struct{}{}}
	if err := s.startAnimationAuthorizationWatch(); err != nil {
		cancel()
		s.animations = nil
		return errors.Join(err, release(), os.RemoveAll(root))
	}
	return nil
}

// CloseAnimations joins workers and pinned transfers before removing private files.
func (s *Service) CloseAnimations() error {
	a := s.animations
	if a == nil {
		return nil
	}
	a.mu.Lock()
	a.closed = true
	for _, j := range a.jobs {
		if j.deliveryCancellationSource == "" {
			j.deliveryCancellationSource = "service"
		}
		if j.cancellationSource == "" && !j.done {
			j.cancellationSource = "service"
		}
	}
	a.cancel()
	if s.reads != nil {
		s.reads.cache.invalidate(func(string) bool { return true })
	}
	for _, j := range a.jobs {
		j.cancel()
		if j.timer != nil {
			j.timer.Stop()
		}
	}
	a.mu.Unlock()
	a.wg.Wait()
	a.sessionWG.Wait()
	<-a.watchDone
	a.mu.Lock()
	defer a.mu.Unlock()
	var err error
	if a.releaseWorkspace != nil {
		err = a.releaseWorkspace()
		a.releaseWorkspace = nil
	}
	return errors.Join(a.watchErr, a.cleanupErr, err, os.RemoveAll(a.root))
}

func animationHandle() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}
func (s *Service) animationClaims(ctx context.Context, account, artwork int64) (claims, error) {
	info := sdkauth.TokenInfoFromContext(ctx)
	if info == nil {
		return claims{}, errCapability
	}
	grant, _ := info.Extra["grant_id"].(string)
	c := claims{Grant: grant, Account: account, Artwork: artwork, Expires: info.Expiration}
	if account <= 0 || s.authorized(ctx, c) != nil {
		return c, errCapability
	}
	return c, nil
}

// PrepareAnimation returns promptly; a subscriber, rather than the tool request,
// owns the worker lifetime. Every prepare receives its own unguessable handle.
func (s *Service) PrepareAnimation(ctx context.Context, account, artwork int64, format string) (AnimationStatus, error) {
	a := s.animations
	if a == nil {
		return AnimationStatus{}, errors.New("animation service unavailable")
	}
	if format == "" {
		format = "gif"
	}
	if artwork <= 0 || (format != "gif" && format != "apng") {
		return AnimationStatus{}, errors.New("invalid animation request")
	}
	c, err := s.animationClaims(ctx, account, artwork)
	if err != nil {
		return AnimationStatus{}, err
	}
	lease, err := s.session(ctx, account)
	if err != nil {
		return AnimationStatus{}, err
	}
	defer lease.Lease.Close()
	c.Quality = format
	key := contentKey(c, lease, "animation")
	handle, err := animationHandle()
	if err != nil {
		return AnimationStatus{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.ctx.Err() != nil {
		return AnimationStatus{}, errors.New("animation service closed")
	}
	// 会话获取可能等待；发布前不能沿用等待前的授权判断。
	if err := s.authorized(ctx, c); err != nil {
		return AnimationStatus{}, err
	}
	j := a.jobs[key]
	if j != nil && (!time.Now().Before(j.expiry) || j.state == "failed" || j.state == "cancelled" || j.state == "expired") {
		a.invalidateLocked(j, "expired", "expiry")
		j = nil
	}
	if j == nil {
		id, e := animationHandle()
		if e != nil {
			return AnimationStatus{}, e
		}
		dir, e := os.MkdirTemp(a.root, "job-")
		if e != nil {
			return AnimationStatus{}, e
		}
		workerCtx, cancel := context.WithCancel(a.ctx)
		j = &animationJob{key: key, id: id, version: lease.Version, format: format, dir: dir, claim: c, expiry: s.expiry(c, lease), state: "queued", ctx: workerCtx, cancel: cancel, last: time.Now()}
		a.jobs[key] = j
		j.timer = time.AfterFunc(time.Until(j.expiry), func() { a.mu.Lock(); defer a.mu.Unlock(); a.invalidateLocked(j, "expired", "expiry") })
		a.wg.Add(1)
		go func() { defer a.wg.Done(); s.buildAnimation(workerCtx, j) }()
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
	}
	if j.state == "ready" {
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_cache_hit", Bytes: j.size})
	}
	j.subscribers++
	j.last = time.Now()
	a.handles[handle] = j
	return s.animationOutputLocked(j, handle), nil
}

// AnimationSubscription revalidates both grant and current credential generation.
func (s *Service) AnimationSubscription(ctx context.Context, account int64, handle string, release bool) (AnimationStatus, error) {
	a := s.animations
	if a == nil {
		return AnimationStatus{}, errors.New("animation service unavailable")
	}
	c, err := s.animationClaims(ctx, account, 0)
	if err != nil {
		// 已失效的同一授权仍需停止既有任务；外部句柄不能取消其他授权的工作。
		a.mu.Lock()
		if j := a.handles[handle]; ctx.Err() == nil && j != nil && c.Grant != "" && j.claim.Grant == c.Grant && j.claim.Account == account {
			a.invalidateLocked(j, "expired", "authorization")
		}
		a.mu.Unlock()
		return AnimationStatus{}, err
	}
	a.mu.Lock()
	j := a.handles[handle]
	if j == nil {
		a.mu.Unlock()
		return AnimationStatus{State: "expired"}, nil
	}
	if j.claim.Grant != c.Grant || j.claim.Account != account {
		a.mu.Unlock()
		return AnimationStatus{}, errCapability
	}
	a.mu.Unlock()
	session, err := s.session(ctx, account)
	if session.Lease != nil {
		defer session.Lease.Close()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	authorizationErr := s.authorized(ctx, j.claim)
	// 短控制请求取消不是授权失效，不能终止独立订阅持有的任务。
	if requestErr := ctx.Err(); requestErr != nil {
		return AnimationStatus{}, requestErr
	}
	if err != nil || session.Version != j.version || authorizationErr != nil {
		a.invalidateLocked(j, "expired", "authorization")
		return AnimationStatus{}, errCapability
	}
	if !time.Now().Before(j.expiry) {
		a.invalidateLocked(j, "expired", "expiry")
	}
	if release {
		if a.handles[handle] == j {
			delete(a.handles, handle)
			j.subscribers--
		}
		if j.subscribers == 0 && j.state != "ready" {
			a.invalidateLocked(j, "cancelled", "subscription")
		}
		return AnimationStatus{State: "cancelled"}, nil
	}
	j.last = time.Now()
	return s.animationOutputLocked(j, handle), nil
}
func (s *Service) animationOutputLocked(j *animationJob, handle string) AnimationStatus {
	out := AnimationStatus{Handle: handle, State: j.state, Error: j.code, Expires: j.expiry}
	if j.state == "ready" {
		out.AnimationURL = s.animationURL(j, "animation")
		out.PreviewURL = s.animationURL(j, "preview")
		out.MediaOrigin = s.Origin()
		out.MIMEType = "image/" + j.format
		out.Filename = fmt.Sprintf("%d.%s", j.claim.Artwork, j.format)
		out.Size = j.size
	}
	return out
}
func (a *animationState) invalidateLocked(j *animationJob, state, source string) {
	if j.deliveryCancellationSource == "" {
		j.deliveryCancellationSource = source
	}
	if j.cancellationSource == "" && !j.done {
		j.cancellationSource = source
	}
	j.cancel()
	j.state = state
	if state == "expired" {
		for handle, job := range a.handles {
			if job == j {
				delete(a.handles, handle)
			}
		}
		j.subscribers = 0
	}
	if j.timer != nil {
		j.timer.Stop()
	}
	if a.jobs[j.key] == j {
		delete(a.jobs, j.key)
	}
	a.cleanLocked(j)
}
func (a *animationState) cleanLocked(j *animationJob) {
	if !j.done || j.pins != 0 || (j.state == "ready" && time.Now().Before(j.expiry)) {
		return
	}
	if err := os.RemoveAll(j.dir); err == nil {
		a.used -= j.used
		j.used = 0
	} else if a.cleanupErr == nil {
		// 保留首个删除错误与占用预算，最终清理成功也不抹去运行期故障。
		a.cleanupErr = fmt.Errorf("animation temporary cleanup: %w", err)
	}
}
func (a *animationState) reserveLocked(j *animationJob, n int64) error {
	if n < 0 || n > a.capacity {
		return errAnimationCapacity
	}
	for a.used > a.capacity-n {
		var oldest *animationJob
		for _, candidate := range a.jobs {
			if candidate != j && candidate.done && candidate.state == "ready" && candidate.subscribers == 0 && candidate.pins == 0 && (oldest == nil || candidate.last.Before(oldest.last)) {
				oldest = candidate
			}
		}
		if oldest == nil {
			return errAnimationCapacity
		}
		a.invalidateLocked(oldest, "expired", "eviction")
	}
	a.used += n
	j.used += n
	return nil
}
func (s *Service) animationStage(j *animationJob, state string) error {
	a := s.animations
	a.mu.Lock()
	defer a.mu.Unlock()
	if j.state == "cancelled" || j.state == "expired" || a.ctx.Err() != nil {
		return context.Canceled
	}
	j.state = state
	return nil
}
func animationFile(dir, name string) (*os.File, error) {
	return os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
}
