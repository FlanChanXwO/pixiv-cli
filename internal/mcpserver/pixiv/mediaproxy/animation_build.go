package mediaproxy

import (
	"context"
	"errors"
	"image"
	"image/gif"
	"image/png"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type animationWriter struct {
	ctx     context.Context
	service *Service
	job     *animationJob
	file    *os.File
}

func (w animationWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	a := w.service.animations
	a.mu.Lock()
	err := a.reserveLocked(w.job, int64(len(data)))
	a.mu.Unlock()
	if err != nil {
		return 0, err
	}
	n, err := w.file.Write(data)
	a.mu.Lock()
	unused := int64(len(data) - n)
	a.used -= unused
	w.job.used -= unused
	a.mu.Unlock()
	return n, err
}
func (s *Service) buildAnimation(ctx context.Context, j *animationJob) {
	a := s.animations
	// 串行暂存生产者，避免排队任务耗尽当前编码所需空间。
	started := time.Now()
	var err error
	// 清理结束后在锁外通知，保留首个取消来源且不输出任务标识。
	defer func() {
		a.mu.Lock()
		source := j.cancellationSource
		if source == "" && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			source = "service"
		}
		failed := err != nil || source != ""
		var size int64
		if !failed {
			size = j.size
		}
		a.mu.Unlock()
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_generation", Duration: time.Since(started), Bytes: size, Failed: failed, CancellationSource: source})
	}()
	select {
	case a.buildGate <- struct{}{}:
		defer func() { <-a.buildGate }()
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_queue_wait", Duration: time.Since(started)})
		err = s.generateAnimation(ctx, j)
	case <-ctx.Done():
		err = ctx.Err()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	j.done = true
	if j.state == "cancelled" || j.state == "expired" {
		a.cleanLocked(j)
		return
	}
	if err != nil {
		j.state = "failed"
		j.code = "animation_generation_failed"
		if errors.Is(err, errAnimationCapacity) || errors.Is(err, ugoira.ErrOutputCapacity) {
			j.code = "animation_capacity_exhausted"
		}
		if errors.Is(err, context.Canceled) {
			j.state = "cancelled"
			j.code = "animation_cancelled"
		}
		a.cleanLocked(j)
		return
	}
	j.state = "ready"
	s.observeAnimation(ctx, AnimationObservation{Stage: "animation_ready", Bytes: j.size})
}
func (s *Service) animationSessionValid(ctx context.Context, j *animationJob) error {
	if s.authorized(ctx, j.claim) != nil {
		return errCapability
	}
	session, err := s.session(ctx, j.claim.Account)
	if session.Lease != nil {
		defer session.Lease.Close()
	}
	if err != nil {
		return err
	}
	if session.Version != j.version {
		return errCapability
	}
	return nil
}
func (s *Service) generateAnimation(ctx context.Context, j *animationJob) error {
	session, err := s.session(ctx, j.claim.Account)
	if err != nil {
		return err
	}
	defer session.Lease.Close()
	if session.Version != j.version {
		return errCapability
	}
	artwork, err := s.metadata(ctx, j.claim, session)
	if err != nil {
		return err
	}
	if artwork.Kind != pixiv.ArtworkKindUgoira {
		return errors.New("artwork is not an animation")
	}
	started := time.Now()
	meta, err := session.Lease.Value().UgoiraMetadata(ctx, pixiv.UgoiraMetadataRequest{ArtworkID: j.claim.Artwork})
	s.observeAnimation(ctx, AnimationObservation{Stage: "animation_metadata", Duration: time.Since(started), Failed: err != nil})
	if err != nil {
		return err
	}
	archive := downloader.SelectUgoiraArchive(meta)
	if archive == nil {
		return errors.New("animation archive unavailable")
	}
	if err = s.animationStage(j, "downloading"); err != nil {
		return err
	}
	zipPath := filepath.Join(j.dir, "frames.zip")
	file, err := animationFile(j.dir, "frames.zip")
	if err != nil {
		return err
	}
	started = time.Now()
	resource, err := session.Lease.Value().OpenResource(ctx, sdk.OpenResourceRequest{Ref: archive.Resource.Ref})
	if err != nil {
		_ = file.Close()
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_download", Duration: time.Since(started), Failed: true})
		return err
	}
	if resource.StatusCode != 200 {
		_ = resource.Body.Close()
		_ = file.Close()
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_download", Duration: time.Since(started), Failed: true})
		return errors.New("animation archive unavailable")
	}
	n, copyErr := io.Copy(animationWriter{ctx, s, j, file}, resource.Body)
	closeErr := errors.Join(resource.Body.Close(), file.Close())
	err = errors.Join(copyErr, closeErr)
	if length := resource.Header().Get("Content-Length"); err == nil && length != "" {
		expected, e := strconv.ParseInt(length, 10, 64)
		if e != nil || expected != n {
			err = errors.Join(errors.New("incomplete animation archive"), io.ErrUnexpectedEOF)
		}
	}
	s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_download", Duration: time.Since(started), Bytes: n, Failed: err != nil})
	if err != nil {
		// 只记录固定类别，避免底层读取错误携带签名地址。
		var timeout net.Error
		if ctx.Err() == nil && errors.Is(err, io.ErrUnexpectedEOF) {
			s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_incomplete", Bytes: n, Failed: true})
		} else if ctx.Err() == nil && errors.Is(err, syscall.ECONNRESET) {
			s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_reset", Bytes: n, Failed: true})
		} else if ctx.Err() == nil && errors.As(err, &timeout) && timeout.Timeout() {
			s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_timeout", Bytes: n, Failed: true})
		}
		return err
	}
	if err = s.animationSessionValid(ctx, j); err != nil {
		return err
	}
	frames := make([]ugoira.Frame, len(meta.Frames))
	for i, frame := range meta.Frames {
		frames[i] = ugoira.Frame{File: frame.Filename, Delay: frame.DelayMilliseconds}
	}
	if err = s.animationStage(j, "encoding"); err != nil {
		return err
	}
	// Reserve actual writes rather than the maximum output: spare capacity must retain ready artifacts.
	a := s.animations
	limit := uint64(a.capacity)
	var reserved int64
	reserveOutput := func(bytes uint64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if bytes > uint64(a.capacity) {
			return errAnimationCapacity
		}
		if err := a.reserveLocked(j, int64(bytes)); err != nil {
			return err
		}
		reserved += int64(bytes)
		return nil
	}
	output := filepath.Join(j.dir, "animation."+j.format)
	var waitDuration, encodeDuration time.Duration
	var waitObserved, encodeObserved bool
	err = ugoira.NewEncoder().Encode(ctx, ugoira.Input{ZipPath: zipPath, Frames: frames, WorkDir: j.dir, OutputPath: output, Format: ugoira.Format(j.format), MaxOutputBytes: &limit, ReserveOutput: reserveOutput, Observe: func(stage string, duration time.Duration) {
		switch stage {
		case "animation_encode_wait":
			waitDuration, waitObserved = duration, true
		case "animation_encode":
			encodeDuration, encodeObserved = duration, true
		}
	}})
	if waitObserved {
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_encode_wait", Duration: waitDuration})
	}
	if encodeObserved {
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_encode", Duration: encodeDuration, Failed: err != nil})
	}
	var size int64
	info, statErr := os.Stat(output)
	if statErr == nil {
		size = info.Size()
	}
	if err == nil {
		err = statErr
	}
	a.mu.Lock()
	// 失败时保留预留，直到目录确实删除；删除故障不能隐藏磁盘占用。
	if err == nil {
		j.frames = len(frames)
		a.used -= reserved - size
		j.used -= reserved - size
	}
	j.size = size
	a.mu.Unlock()
	if err != nil {
		return err
	}
	if err = os.Remove(zipPath); err != nil {
		return err
	}
	a.mu.Lock()
	a.used -= n
	j.used -= n
	a.mu.Unlock()
	encoded, err := os.Open(output)
	if err != nil {
		return err
	}
	var first image.Image
	if j.format == "gif" {
		first, err = gif.Decode(encoded)
	} else {
		first, err = png.Decode(encoded)
	}
	err = errors.Join(err, encoded.Close())
	if err != nil {
		return err
	}
	preview, err := animationFile(j.dir, "preview.png")
	if err != nil {
		return err
	}
	err = png.Encode(animationWriter{ctx, s, j, preview}, first)
	err = errors.Join(err, preview.Close())
	if err != nil {
		return err
	}
	return s.animationSessionValid(ctx, j)
}

func (s *Service) observeAnimation(ctx context.Context, event AnimationObservation) {
	if s.reads == nil {
		return
	}
	if s.reads.config.ObserveAnimation != nil {
		s.reads.config.ObserveAnimation(ctx, event)
	} else if s.reads.config.Observe != nil {
		s.reads.config.Observe(ctx, event.Stage, event.Duration)
	}
}
