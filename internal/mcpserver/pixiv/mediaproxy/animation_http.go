// Authenticate animation artifact requests and pin files through HTTP delivery.

package mediaproxy

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (s *Service) animationURL(j *animationJob, kind string) string {
	payload := j.id + "." + kind
	return s.base + "/media/animation/" + payload + "." + base64.RawURLEncoding.EncodeToString(s.signCapability(payload))
}
func (s *Service) serveAnimation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	kind, j, download, ok := s.animationRequest(w, r)
	if !ok {
		return
	}
	release, ok := s.pinAnimation(w, r, j)
	if !ok {
		return
	}
	defer release()
	s.deliverAnimation(w, r, j, kind, download)
}

// animationRequest 校验方法、路径签名，并返回已签发的产物及其类别。
func (s *Service) animationRequest(w http.ResponseWriter, r *http.Request) (kind string, j *animationJob, download, ok bool) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", 405)
		return "", nil, false, false
	}
	a := s.animations
	parts := strings.Split(strings.TrimPrefix(r.URL.EscapedPath(), s.Path()+"animation/"), ".")
	download = r.URL.RawQuery == "download=1"
	if a == nil || len(parts) != 3 || (r.URL.RawQuery != "" && !download) || r.URL.ForceQuery || (parts[1] != "animation" && parts[1] != "preview") || (download && parts[1] != "animation") {
		http.Error(w, "Invalid animation request", 400)
		return "", nil, false, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, s.signCapability(parts[0]+"."+parts[1])) {
		http.Error(w, "Animation authorization unavailable", 401)
		return "", nil, false, false
	}
	a.mu.Lock()
	for _, candidate := range a.jobs {
		if candidate.id == parts[0] {
			j = candidate
			break
		}
	}
	a.mu.Unlock()
	if j == nil {
		http.Error(w, "Animation expired", 410)
		return "", nil, false, false
	}
	return parts[1], j, download, true
}

// pinAnimation 复核授权与有效期后固定产物，返回的释放函数在传输结束时调用。
func (s *Service) pinAnimation(w http.ResponseWriter, r *http.Request, j *animationJob) (release func(), ok bool) {
	a := s.animations
	if err := s.animationSessionValid(r.Context(), j); err != nil {
		a.mu.Lock()
		// 单次传输取消不代表共享产物的授权或账号版本失效。
		if r.Context().Err() == nil {
			a.invalidateLocked(j, "expired", "authorization")
		}
		a.mu.Unlock()
		http.Error(w, "Animation authorization unavailable", 401)
		return nil, false
	}
	a.mu.Lock()
	if a.closed || j.state != "ready" || !time.Now().Before(j.expiry) {
		a.mu.Unlock()
		http.Error(w, "Animation unavailable", 410)
		return nil, false
	}
	j.pins++
	j.last = time.Now()
	a.wg.Add(1)
	a.mu.Unlock()
	return func() {
		a.mu.Lock()
		j.pins--
		a.cleanLocked(j)
		a.mu.Unlock()
		a.wg.Done()
	}, true
}

// deliverAnimation 提交头部并复制已固定的产物；取消或长度不符时中止连接。
func (s *Service) deliverAnimation(w http.ResponseWriter, r *http.Request, j *animationJob, kind string, download bool) {
	name := "animation." + j.format
	contentType := "image/" + j.format
	if kind == "preview" {
		name = "preview.png"
		contentType = "image/png"
	}
	file, err := os.Open(filepath.Join(j.dir, name))
	if err != nil {
		http.Error(w, "Animation unavailable", 502)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "Animation unavailable", 502)
		return
	}
	if s.authorized(r.Context(), j.claim) != nil {
		http.Error(w, "Animation authorization unavailable", 401)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	if download {
		w.Header().Set("Content-Disposition", `attachment; filename="`+strconv.FormatInt(j.claim.Artwork, 10)+"."+j.format+`"`)
	}
	w.WriteHeader(200)
	if r.Method == "HEAD" {
		return
	}
	s.transferAnimation(w, r, j, file, info.Size())
}

// transferAnimation 在请求或产物失效时中断写入，并记录实际交付字节。
func (s *Service) transferAnimation(w http.ResponseWriter, r *http.Request, j *animationJob, file *os.File, size int64) {
	transferCtx, cancel := context.WithCancel(r.Context())
	stopJob := context.AfterFunc(j.ctx, cancel)
	defer stopJob()
	defer cancel()
	aborted := make(chan struct{})
	var abortErr error
	stop := context.AfterFunc(transferCtx, func() {
		defer close(aborted)
		// 已取消才中断写入；仅关闭文件不能解除阻塞的网络 Write。
		abortErr = errors.Join(http.NewResponseController(w).SetWriteDeadline(time.Now()), file.Close())
	})
	finish := sync.OnceFunc(func() {
		if !stop() {
			<-aborted
		}
	})
	// callback 必须在 handler 返回前结束，不能再操作被复用的 ResponseWriter。
	defer finish()
	started := time.Now()
	n, err := io.Copy(w, file)
	finish()
	err = errors.Join(err, abortErr, transferCtx.Err())
	event := AnimationObservation{Stage: "animation_delivery", Duration: time.Since(started), Bytes: n}
	if err != nil || n != size {
		s.animationCancellationSource(r.Context(), transferCtx, j, &event)
	}
	s.observeAnimation(r.Context(), event)
	if err != nil || n != size {
		panic(http.ErrAbortHandler)
	}
}

// animationCancellationSource 记录固定类别，便于区分请求、传输、产物与服务侧中断。
func (s *Service) animationCancellationSource(requestCtx, transferCtx context.Context, j *animationJob, event *AnimationObservation) {
	event.CancellationSource = "transport"
	if requestCtx.Err() != nil {
		event.CancellationSource = "request"
		return
	}
	if transferCtx.Err() == nil {
		return
	}
	a := s.animations
	a.mu.Lock()
	event.CancellationSource = j.deliveryCancellationSource
	a.mu.Unlock()
	if event.CancellationSource == "" {
		event.CancellationSource = "service"
	}
}
