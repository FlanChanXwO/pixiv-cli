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

// Write 检查任务 context 仍有效后写入暂存文件，并累计实际字节数。
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
	// 生产者串行暂存，避免排队任务把当前编码需要的空间耗尽。
	started := time.Now()
	var err error
	// 清理完再在锁外通知，保留第一个取消来源且不输出任务标识。
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
	session, err := s.animationSourceSession(ctx, j)
	if err != nil {
		return err
	}
	defer session.Lease.Close()
	meta, archiveBytes, err := s.downloadAnimationArchive(ctx, j, session)
	if err != nil {
		return err
	}
	return s.encodeAnimationPreview(ctx, j, session, meta, archiveBytes)
}

// animationSourceSession 确认账号版本未变，并取得该作品的元数据快照。
func (s *Service) animationSourceSession(ctx context.Context, j *animationJob) (Session, error) {
	session, err := s.session(ctx, j.claim.Account)
	if err != nil {
		return Session{}, err
	}
	if session.Version != j.version {
		session.Lease.Close()
		return Session{}, errCapability
	}
	artwork, err := s.metadata(ctx, j.claim, session)
	if err != nil {
		session.Lease.Close()
		return Session{}, err
	}
	if artwork.ContentType != pixiv.ContentTypeUgoira {
		session.Lease.Close()
		return Session{}, errors.New("artwork is not an animation")
	}
	return session, nil
}

// downloadAnimationArchive 读取 ugoira 元数据，把帧归档写入私有工作目录。
func (s *Service) downloadAnimationArchive(ctx context.Context, j *animationJob, session Session) (pixiv.UgoiraMetadata, int64, error) {
	started := time.Now()
	meta, err := session.Lease.Value().UgoiraMetadata(ctx, pixiv.UgoiraMetadataRequest{ArtworkID: j.claim.Artwork})
	s.observeAnimation(ctx, AnimationObservation{Stage: "animation_metadata", Duration: time.Since(started), Failed: err != nil})
	if err != nil {
		return pixiv.UgoiraMetadata{}, 0, err
	}
	archive := downloader.SelectUgoiraArchive(meta)
	if archive == nil {
		return pixiv.UgoiraMetadata{}, 0, errors.New("animation archive unavailable")
	}
	if err = s.animationStage(j, "downloading"); err != nil {
		return pixiv.UgoiraMetadata{}, 0, err
	}
	file, err := animationFile(j.dir, "frames.zip")
	if err != nil {
		return pixiv.UgoiraMetadata{}, 0, err
	}
	started = time.Now()
	resource, err := session.Lease.Value().OpenResource(ctx, sdk.OpenResourceRequest{Ref: archive.Resource.Ref})
	if err != nil {
		_ = file.Close()
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_download", Duration: time.Since(started), Failed: true})
		return pixiv.UgoiraMetadata{}, 0, err
	}
	if resource.StatusCode != 200 {
		_ = resource.Body.Close()
		_ = file.Close()
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_download", Duration: time.Since(started), Failed: true})
		return pixiv.UgoiraMetadata{}, 0, errors.New("animation archive unavailable")
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
		s.observeArchiveFailure(ctx, n, err)
		return pixiv.UgoiraMetadata{}, 0, err
	}
	if err = s.animationSessionValid(ctx, j); err != nil {
		return pixiv.UgoiraMetadata{}, 0, err
	}
	return meta, n, nil
}

// observeArchiveFailure 只记录固定类别，避免底层读取错误带出签名地址。
func (s *Service) observeArchiveFailure(ctx context.Context, n int64, err error) {
	var timeout net.Error
	switch {
	case ctx.Err() != nil:
	case errors.Is(err, io.ErrUnexpectedEOF):
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_incomplete", Bytes: n, Failed: true})
	case errors.Is(err, syscall.ECONNRESET):
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_reset", Bytes: n, Failed: true})
	case errors.As(err, &timeout) && timeout.Timeout():
		s.observeAnimation(ctx, AnimationObservation{Stage: "animation_zip_timeout", Bytes: n, Failed: true})
	}
}

// encodeAnimationPreview 编码请求的格式、生成 PNG 预览，并确认会话仍然有效。
func (s *Service) encodeAnimationPreview(ctx context.Context, j *animationJob, session Session, meta pixiv.UgoiraMetadata, archiveBytes int64) error {
	frames := make([]ugoira.Frame, len(meta.Frames))
	for i, frame := range meta.Frames {
		frames[i] = ugoira.Frame{File: frame.Filename, Delay: frame.DelayMilliseconds}
	}
	if err := s.animationStage(j, "encoding"); err != nil {
		return err
	}
	output := filepath.Join(j.dir, "animation."+j.format)
	if err := s.encodeAnimation(ctx, j, frames, output); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(j.dir, "frames.zip")); err != nil {
		return err
	}
	a := s.animations
	a.mu.Lock()
	a.used -= archiveBytes
	j.used -= archiveBytes
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

// encodeAnimation 按实际输出预留磁盘空间，并记录编码各阶段耗时。
func (s *Service) encodeAnimation(ctx context.Context, j *animationJob, frames []ugoira.Frame, output string) error {
	// 按实际写入预留，不为最大可能输出预留；剩余容量要留给已完成的产物。
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
	var waitDuration, encodeDuration time.Duration
	var waitObserved, encodeObserved bool
	err := ugoira.NewEncoder().Encode(ctx, ugoira.Input{ZipPath: filepath.Join(j.dir, "frames.zip"), Frames: frames, WorkDir: j.dir, OutputPath: output, Format: ugoira.Format(j.format), MaxOutputBytes: &limit, ReserveOutput: reserveOutput, Observe: func(stage string, duration time.Duration) {
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
	// 失败时保留预留直到目录确实删除，磁盘占用始终可见。
	if err == nil {
		j.frames = len(frames)
		a.used -= reserved - size
		j.used -= reserved - size
	}
	j.size = size
	a.mu.Unlock()
	return err
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
