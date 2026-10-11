package mediaproxy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte(payload))
	return s.base + "/media/animation/" + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) serveAnimation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", 405)
		return
	}
	a := s.animations
	parts := strings.Split(strings.TrimPrefix(r.URL.EscapedPath(), s.Path()+"animation/"), ".")
	if a == nil || len(parts) != 3 || r.URL.RawQuery != "" || (parts[1] != "animation" && parts[1] != "preview") {
		http.Error(w, "Invalid animation request", 400)
		return
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		http.Error(w, "Animation authorization unavailable", 401)
		return
	}
	a.mu.Lock()
	var j *animationJob
	for _, candidate := range a.jobs {
		if candidate.id == parts[0] {
			j = candidate
			break
		}
	}
	a.mu.Unlock()
	if j == nil {
		http.Error(w, "Animation expired", 410)
		return
	}
	if err = s.animationSessionValid(r.Context(), j); err != nil {
		a.mu.Lock()
		// 单次传输取消不代表共享产物的授权或账号版本失效。
		if r.Context().Err() == nil {
			a.invalidateLocked(j, "expired", "authorization")
		}
		a.mu.Unlock()
		http.Error(w, "Animation authorization unavailable", 401)
		return
	}
	a.mu.Lock()
	if a.closed || j.state != "ready" || !time.Now().Before(j.expiry) {
		a.mu.Unlock()
		http.Error(w, "Animation unavailable", 410)
		return
	}
	j.pins++
	j.last = time.Now()
	a.wg.Add(1)
	a.mu.Unlock()
	defer func() { a.mu.Lock(); j.pins--; a.cleanLocked(j); a.mu.Unlock(); a.wg.Done() }()
	name := "animation." + j.format
	mime := "image/" + j.format
	if parts[1] == "preview" {
		name = "preview.png"
		mime = "image/png"
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
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.WriteHeader(200)
	if r.Method == "HEAD" {
		return
	}
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
	if err != nil || n != info.Size() {
		event.CancellationSource = "transport"
		if r.Context().Err() != nil {
			event.CancellationSource = "request"
		} else if transferCtx.Err() != nil {
			a.mu.Lock()
			event.CancellationSource = j.deliveryCancellationSource
			a.mu.Unlock()
			if event.CancellationSource == "" {
				event.CancellationSource = "service"
			}
		}
	}
	s.observeAnimation(r.Context(), event)
	if err != nil || n != info.Size() {
		panic(http.ErrAbortHandler)
	}
}
