package mediaproxy

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/iotest"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	fileatomic "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/atomic"
	"github.com/stretchr/testify/require"
)

func TestAnimationFailedPhasesAreNotReportedAsSuccess(t *testing.T) {
	for _, tc := range []struct {
		stage, marker string
		readErr       error
		length        string
	}{
		{stage: "animation_metadata"},
		{stage: "animation_zip_download", marker: "animation_zip_incomplete", readErr: io.ErrUnexpectedEOF},
		{stage: "animation_zip_download", marker: "animation_zip_incomplete", length: "28"},
		{stage: "animation_zip_download", marker: "animation_zip_reset", readErr: syscall.ECONNRESET},
		{stage: "animation_zip_download", marker: "animation_zip_timeout", readErr: &url.Error{Op: "Get", URL: "https://i.pximg.net/fixture-private?signature=fixture-private", Err: os.ErrDeadlineExceeded}},
		{stage: "animation_encode"},
	} {
		stage := tc.stage
		t.Run(stage+"/"+tc.marker, func(t *testing.T) {
			var opened, closed atomic.Int32
			s, _, _, _ := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
				status := 200
				header := http.Header{}
				var body io.Reader
				switch {
				case r.URL.Host == "i.pximg.net":
					body = strings.NewReader("fixture-private-corrupt-zip")
					if tc.readErr != nil {
						body = io.MultiReader(body, iotest.ErrReader(tc.readErr))
					}
					if tc.length != "" {
						header.Set("Content-Length", tc.length)
					}
				case strings.Contains(r.URL.Path, "ugoira"):
					body = strings.NewReader(`{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/frames.zip"},"frames":[{"file":"0.png","delay":80}]}}`)
					if stage == "animation_metadata" {
						status = 503
					}
				default:
					body = strings.NewReader(`{"illust":{"id":42,"title":"animation","type":"ugoira","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}}`)
				}
				opened.Add(1)
				return &http.Response{StatusCode: status, Header: header, Body: animationObservedBody{body, &closed}}, nil
			}, 1<<20)
			var mu sync.Mutex
			var events []AnimationObservation
			s.reads.config.ObserveAnimation = func(_ context.Context, event AnimationObservation) {
				mu.Lock()
				defer mu.Unlock()
				events = append(events, event)
			}
			require.NoError(t, s.ConfigureAnimations(t.Context(), 1<<20))
			t.Cleanup(func() { require.NoError(t, s.CloseAnimations()) })
			ctx := animationContext(t, s)
			prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
			require.NoError(t, err)
			failed := awaitAnimation(t, s, ctx, prepared.Handle)
			require.Equal(t, "failed", failed.State)
			require.Empty(t, failed.AnimationURL, "failed jobs must not publish incomplete artifacts")
			// 等待 worker 收尾后再读观察结果，不将 status ready/failed 当作观察回调已完成。
			s.animations.wg.Wait()
			files, err := os.ReadDir(s.animations.root)
			require.NoError(t, err)
			require.Len(t, files, 2, "failed downloads must remove their private workspace")
			require.Zero(t, s.animations.used, "failed downloads must release the reserved budget")
			require.Equal(t, opened.Load(), closed.Load(), "failed reads must close every upstream body")
			mu.Lock()
			raw, err := json.Marshal(events)
			mu.Unlock()
			require.NoError(t, err)
			var observed []map[string]any
			require.NoError(t, json.Unmarshal(raw, &observed))
			found := map[string]map[string]any{}
			for _, event := range observed {
				found[event["Stage"].(string)] = event
			}
			require.Contains(t, found, stage)
			require.Equal(t, true, found[stage]["Failed"], "failed phase must not be reported as success")
			if tc.marker != "" {
				require.Contains(t, found, tc.marker, "failed archive reads need a safe diagnostic marker")
				require.Equal(t, true, found[tc.marker]["Failed"])
				require.Equal(t, float64(27), found[tc.marker]["Bytes"], "report only the bytes received before failure")
			}
			require.Contains(t, found, "animation_generation")
			require.Equal(t, true, found["animation_generation"]["Failed"])
			require.Equal(t, float64(0), found["animation_generation"]["Bytes"])
			require.NotContains(t, string(raw), "fixture-private")
			require.NotContains(t, string(raw), "pximg.net")
			require.NotContains(t, string(raw), s.animations.root)
			before := opened.Load()
			retried, err := s.PrepareAnimation(ctx, 72, 42, "gif")
			require.NoError(t, err)
			require.NotEqual(t, prepared.Handle, retried.Handle)
			require.Equal(t, "failed", awaitAnimation(t, s, ctx, retried.Handle).State)
			s.animations.wg.Wait()
			require.Greater(t, opened.Load(), before, "explicit prepare must retry upstream, not cache a failed result")
			require.Equal(t, opened.Load(), closed.Load())
		})
	}
}

func TestAnimationReportsNativePhases(t *testing.T) {
	s, ctx, _, _, _ := animationProxy(t, 1<<20)
	var mu sync.Mutex
	phases := map[string]time.Duration{}
	s.reads.config.ObserveAnimation = func(_ context.Context, event AnimationObservation) {
		mu.Lock()
		defer mu.Unlock()
		phases[event.Stage] = event.Duration
	}
	prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	require.Equal(t, "ready", awaitAnimation(t, s, ctx, prepared.Handle).State)
	s.animations.wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	for _, stage := range []string{"animation_encode_wait", "animation_encode"} {
		duration, observed := phases[stage]
		require.True(t, observed, "zero duration must not hide an observed phase: %s", stage)
		require.GreaterOrEqual(t, duration, time.Duration(0))
	}
}

type animationObservedBody struct {
	io.Reader
	closed *atomic.Int32
}

func (b animationObservedBody) Close() error { b.closed.Add(1); return nil }

func animationContext(t *testing.T, s *Service) context.Context {
	t.Helper()
	state, err := s.store.Read(t.Context())
	require.NoError(t, err)
	g := state.Grants["g"]
	h := sha256.Sum256([]byte("animation-fixture"))
	g.AccessTokens[hex.EncodeToString(h[:])] = time.Now().Add(time.Hour)
	state.Grants["g"] = g
	data, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(s.store.Path, data, 0600))
	handler, err := auth.NewHandler(s.base, s.store)
	require.NoError(t, err)
	var ctx context.Context
	handler.RequireBearer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { ctx = r.Context() })).ServeHTTP(httptest.NewRecorder(), func() *http.Request {
		r := httptest.NewRequest("POST", s.base+"/mcp", nil)
		r.Header.Set("Authorization", "Bearer animation-fixture")
		return r
	}())
	require.NotNil(t, ctx)
	return ctx
}

func TestAnimationGrantReplayInvalidatesOnlyItsReadyArtifact(t *testing.T) {
	s, ctx, _, _, store := animationProxy(t, 1<<20)
	state, err := store.Read(t.Context())
	require.NoError(t, err)
	replay := sha256.Sum256([]byte("replayed-fixture"))
	grant := state.Grants["g"]
	grant.UsedRefreshHashes = []string{hex.EncodeToString(replay[:])}
	state.Grants["g"] = grant
	other := grant
	access := sha256.Sum256([]byte("other-fixture"))
	other.AccessTokens = map[string]time.Time{hex.EncodeToString(access[:]): time.Now().Add(time.Hour)}
	other.UsedRefreshHashes = []string{}
	state.Grants["other"] = other
	data, err := json.Marshal(state)
	require.NoError(t, err)
	_, err = fileatomic.AtomicWrite(t.Context(), store.Path, bytes.NewReader(data))
	require.NoError(t, err)
	handler, err := auth.NewHandler(s.base, store)
	require.NoError(t, err)
	var otherCtx context.Context
	request := httptest.NewRequest("POST", s.base+"/mcp", nil)
	request.Header.Set("Authorization", "Bearer other-fixture")
	handler.RequireBearer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { otherCtx = r.Context() })).ServeHTTP(httptest.NewRecorder(), request)
	require.NotNil(t, otherCtx)
	first, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	ready := awaitAnimation(t, s, ctx, first.Handle)
	require.Equal(t, "ready", ready.State)
	require.Equal(t, 2, ready.FrameCount, "UI cache estimate uses the encoded source frame count")
	wire, err := json.Marshal(ready)
	require.NoError(t, err)
	require.NotContains(t, string(wire), "FrameCount", "UI hint must not change structured output")
	second, err := s.PrepareAnimation(otherCtx, 72, 42, "gif")
	require.NoError(t, err)
	otherReady := awaitAnimation(t, s, otherCtx, second.Handle)
	require.Equal(t, "ready", otherReady.State)
	require.NotEqual(t, ready.AnimationURL, otherReady.AnimationURL)
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {"c"}, "resource": {s.base + "/mcp"}, "refresh_token": {"replayed-fixture"}}
	request = httptest.NewRequest("POST", s.base+"/oauth/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)
	// 不读取撤销grant的媒体来触发清理；目录仅保留另一个grant的产物。
	require.Eventually(t, func() bool {
		files, err := os.ReadDir(s.animations.root)
		return err == nil && len(files) == 3
	}, 3*time.Second, time.Millisecond)
	status, err := s.AnimationSubscription(otherCtx, 72, second.Handle, false)
	require.NoError(t, err)
	require.Equal(t, "ready", status.State)
	get := httptest.NewRecorder()
	s.ServeHTTP(get, httptest.NewRequest("GET", otherReady.AnimationURL, nil))
	require.Equal(t, http.StatusOK, get.Code)
	frames, err := gif.DecodeAll(bytes.NewReader(get.Body.Bytes()))
	require.NoError(t, err)
	require.Len(t, frames.Image, 2)
}

func TestAnimationPreparationReturnsShortIndependentSubscriptions(t *testing.T) {
	s, ctx, _, _, _ := animationProxy(t, 1<<20)
	first, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	// 使用有效动图并等待完成，避免失败重建清理旧句柄而绕过隔离断言。
	s.animations.wg.Wait()
	second, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	require.Equal(t, "queued", first.State)
	require.Equal(t, "ready", second.State)
	require.NotEmpty(t, first.Handle)
	require.NotEqual(t, first.Handle, second.Handle)
	require.Empty(t, first.AnimationURL, "binary must not be embedded in prepare")
	_, err = s.AnimationSubscription(ctx, 73, first.Handle, false)
	require.Error(t, err, "foreign account must not access handles")
}

func TestAnimationPreparationRejectsGrantRevokedDuringSessionAcquisition(t *testing.T) {
	s, _, _, store := newCachedProxyFixture(t, func(*http.Request) (*http.Response, error) {
		return fixtureArtwork("42"), nil
	}, 1<<20)
	open := s.reads.open
	require.NoError(t, s.Configure(s.reads.config, func(requestCtx context.Context, account int64) (Session, error) {
		lease, err := open(requestCtx, account)
		if err != nil {
			return lease, err
		}
		_, resetErr := store.Init(requestCtx, true)
		if resetErr != nil {
			lease.Lease.Close()
			return Session{}, resetErr
		}
		return lease, nil
	}))
	require.NoError(t, s.ConfigureAnimations(t.Context(), 1<<20))
	t.Cleanup(func() { require.NoError(t, s.CloseAnimations()) })
	ctx := animationContext(t, s)
	prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.Error(t, err, "revocation before publication must reject the prepare request")
	require.Empty(t, prepared.Handle)
}

func TestAnimationCloseReportsEarlierArtifactCleanupFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires POSIX directory permission enforcement")
	}
	data := animationArchive(t)
	s, _, _, store := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
		var body io.Reader
		switch {
		case r.URL.Host == "i.pximg.net":
			body = bytes.NewReader(data)
		case strings.Contains(r.URL.Path, "ugoira"):
			body = strings.NewReader(`{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/frames.zip"},"frames":[{"file":"0.png","delay":80},{"file":"1.png","delay":120}]}}`)
		default:
			body = strings.NewReader(`{"illust":{"id":42,"title":"animation","type":"ugoira","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}}`)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(body)}, nil
	}, 1<<20)
	require.NoError(t, s.ConfigureAnimations(t.Context(), 1<<20))
	defer s.CloseAnimations()
	ctx := animationContext(t, s)
	prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	require.Equal(t, "ready", awaitAnimation(t, s, ctx, prepared.Handle).State)
	entries, err := os.ReadDir(s.animations.root)
	require.NoError(t, err)
	var directory string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "job-") {
			directory = filepath.Join(s.animations.root, entry.Name())
		}
	}
	require.NotEmpty(t, directory)
	require.NoError(t, os.Chmod(directory, 0500))
	defer os.Chmod(directory, 0700)
	_, err = store.Init(t.Context(), true)
	require.NoError(t, err)
	_, err = s.AnimationSubscription(ctx, 72, prepared.Handle, false)
	require.Error(t, err)
	// 最终清理恢复成功也不能掩盖运行期间已发生的删除失败。
	require.NoError(t, os.Chmod(directory, 0700))
	require.ErrorIs(t, s.CloseAnimations(), os.ErrPermission)
}

func animationArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for i, c := range []color.RGBA{{R: 255, A: 255}, {B: 255, A: 255}} {
		f, err := z.Create(fmt.Sprintf("%d.png", i))
		require.NoError(t, err)
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				img.SetRGBA(x, y, c)
			}
		}
		require.NoError(t, png.Encode(f, img))
	}
	require.NoError(t, z.Close())
	return buf.Bytes()
}
func animationProxy(t *testing.T, capacity int64) (*Service, context.Context, *atomic.Int32, *atomic.Uint64, auth.Store) {
	data := animationArchive(t)
	count := &atomic.Int32{}
	s, _, version, store := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
		var body io.ReadCloser
		switch {
		case r.URL.Host == "i.pximg.net":
			count.Add(1)
			body = io.NopCloser(bytes.NewReader(data))
		case strings.Contains(r.URL.Path, "ugoira"):
			body = io.NopCloser(strings.NewReader(`{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/frames.zip"},"frames":[{"file":"0.png","delay":80},{"file":"1.png","delay":120}]}}`))
		default:
			body = io.NopCloser(strings.NewReader(`{"illust":{"id":42,"title":"animation","type":"ugoira","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}}`))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}, nil
	}, 1<<20)
	require.NoError(t, s.ConfigureAnimations(t.Context(), capacity))
	t.Cleanup(func() { require.NoError(t, s.CloseAnimations()) })
	return s, animationContext(t, s), count, version, store
}
func awaitAnimation(t *testing.T, s *Service, ctx context.Context, handle string) AnimationStatus {
	t.Helper()
	var out AnimationStatus
	require.Eventually(t, func() bool {
		var err error
		out, err = s.AnimationSubscription(ctx, 72, handle, false)
		require.NoError(t, err)
		return out.State == "ready" || out.State == "failed" || out.State == "cancelled"
	}, 5*time.Second, time.Millisecond)
	return out
}
func TestAnimationMergingProtectedDeliveryAndVersionInvalidation(t *testing.T) {
	for _, format := range []string{"gif", "apng"} {
		t.Run(format, func(t *testing.T) {
			s, ctx, count, version, _ := animationProxy(t, 1<<20)
			first, err := s.PrepareAnimation(ctx, 72, 42, format)
			require.NoError(t, err)
			second, err := s.PrepareAnimation(ctx, 72, 42, format)
			require.NoError(t, err)
			_, err = s.AnimationSubscription(ctx, 72, first.Handle, true)
			require.NoError(t, err)
			ready := awaitAnimation(t, s, ctx, second.Handle)
			require.Equal(t, "ready", ready.State)
			require.EqualValues(t, 1, count.Load())
			repeat, err := s.PrepareAnimation(ctx, 72, 42, format)
			require.NoError(t, err)
			require.Equal(t, "ready", repeat.State)
			require.EqualValues(t, 1, count.Load())
			for _, address := range []string{ready.AnimationURL, ready.PreviewURL} {
				get := httptest.NewRecorder()
				s.ServeHTTP(get, httptest.NewRequest("GET", address, nil))
				require.Equal(t, 200, get.Code)
				require.Equal(t, "private, no-store", get.Header().Get("Cache-Control"))
				if address == ready.PreviewURL {
					_, err = png.Decode(bytes.NewReader(get.Body.Bytes()))
				} else if format == "gif" {
					var decoded *gif.GIF
					decoded, err = gif.DecodeAll(bytes.NewReader(get.Body.Bytes()))
					if err == nil {
						require.Len(t, decoded.Image, 2)
					}
				} else {
					require.Contains(t, string(get.Body.Bytes()), "acTL")
					_, err = png.Decode(bytes.NewReader(get.Body.Bytes()))
				}
				require.NoError(t, err)
				head := httptest.NewRecorder()
				s.ServeHTTP(head, httptest.NewRequest("HEAD", address, nil))
				require.Equal(t, 200, head.Code)
				require.Equal(t, get.Header().Get("Content-Length"), head.Header().Get("Content-Length"))
				require.Empty(t, head.Body.Bytes())
			}
			attachment := httptest.NewRecorder()
			s.ServeHTTP(attachment, httptest.NewRequest("GET", ready.AnimationURL+"?download=1", nil))
			require.Equal(t, 200, attachment.Code)
			require.Equal(t, `attachment; filename="42.`+format+`"`, attachment.Header().Get("Content-Disposition"))
			inline := httptest.NewRecorder()
			s.ServeHTTP(inline, httptest.NewRequest("GET", ready.AnimationURL, nil))
			require.Equal(t, inline.Body.Bytes(), attachment.Body.Bytes())
			require.Empty(t, inline.Header().Get("Content-Disposition"))
			attachmentHead := httptest.NewRecorder()
			s.ServeHTTP(attachmentHead, httptest.NewRequest("HEAD", ready.AnimationURL+"?download=1", nil))
			require.Equal(t, 200, attachmentHead.Code)
			require.Equal(t, attachment.Header().Get("Content-Disposition"), attachmentHead.Header().Get("Content-Disposition"))
			require.Equal(t, attachment.Header().Get("Content-Length"), attachmentHead.Header().Get("Content-Length"))
			require.Empty(t, attachmentHead.Body.Bytes())
			previewDownload := httptest.NewRecorder()
			s.ServeHTTP(previewDownload, httptest.NewRequest("GET", ready.PreviewURL+"?download=1", nil))
			require.Equal(t, 400, previewDownload.Code)
			for _, query := range []string{"download=0", "download=1&extra=1", "download=1&download=1"} {
				invalid := httptest.NewRecorder()
				s.ServeHTTP(invalid, httptest.NewRequest("GET", ready.AnimationURL+"?"+query, nil))
				require.Equal(t, 400, invalid.Code)
			}
			version.Add(1)
			blocked := httptest.NewRecorder()
			s.ServeHTTP(blocked, httptest.NewRequest("GET", ready.AnimationURL, nil))
			require.Equal(t, 401, blocked.Code)
			blockedDownload := httptest.NewRecorder()
			s.ServeHTTP(blockedDownload, httptest.NewRequest("GET", ready.AnimationURL+"?download=1", nil))
			require.Contains(t, []int{401, 410}, blockedDownload.Code)
			files, err := os.ReadDir(s.animations.root)
			require.NoError(t, err)
			require.Equal(t, []string{"instance.lock", "owner"}, animationFileNames(files))
		})
	}
}
func TestAnimationNativeCapacityFailureIsExplicit(t *testing.T) {
	data := animationArchive(t)
	s, ctx, _, _, _ := animationProxy(t, int64(len(data)+16))
	prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	failed := awaitAnimation(t, s, ctx, prepared.Handle)
	require.Equal(t, "failed", failed.State)
	require.Equal(t, "animation_capacity_exhausted", failed.Error)
	files, err := os.ReadDir(s.animations.root)
	require.NoError(t, err)
	require.Equal(t, []string{"instance.lock", "owner"}, animationFileNames(files))
}

func TestAnimationExpiryRevocationAndCleanup(t *testing.T) {
	for _, mode := range []string{"expiry", "revocation", "close"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, _, _, store := animationProxy(t, 1<<20)
			if mode == "expiry" {
				s.reads.config.TTL = 100 * time.Millisecond
			}
			prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
			require.NoError(t, err)
			var ready AnimationStatus
			if mode == "expiry" {
				// 有效期可在生成完成前结束；到期契约不要求编码在100ms内完成。
				require.Eventually(t, func() bool {
					status, err := s.AnimationSubscription(ctx, 72, prepared.Handle, false)
					return err == nil && status.State == "expired"
				}, time.Second, time.Millisecond)
			} else {
				ready = awaitAnimation(t, s, ctx, prepared.Handle)
				require.Equal(t, "ready", ready.State)
			}
			if mode == "revocation" {
				state, err := store.Read(t.Context())
				require.NoError(t, err)
				g := state.Grants["g"]
				g.Revoked = true
				state.Grants["g"] = g
				body, err := json.Marshal(state)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(store.Path, body, 0600))
				response := httptest.NewRecorder()
				s.ServeHTTP(response, httptest.NewRequest("GET", ready.AnimationURL, nil))
				// 通知先移除产物时为410；尚未移除时授权检查返回401。两者都必须拒绝媒体。
				require.Contains(t, []int{http.StatusUnauthorized, http.StatusGone}, response.Code)
				require.NotContains(t, response.Header().Get("Content-Type"), "image/")
			}
			if mode == "close" {
				require.NoError(t, s.CloseAnimations())
				_, err = os.Stat(s.animations.root)
				require.True(t, os.IsNotExist(err))
				return
			}
			require.Eventually(t, func() bool {
				files, err := os.ReadDir(s.animations.root)
				return err == nil && len(files) == 2 && files[0].Name() == "instance.lock" && files[1].Name() == "owner"
			}, time.Second, time.Millisecond)
		})
	}
}

type cancelledAnimationBody struct {
	ctx     context.Context
	entered chan struct{}
	once    sync.Once
	closed  atomic.Bool
}

func (b *cancelledAnimationBody) Read(_ []byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b *cancelledAnimationBody) Close() error { b.closed.Store(true); return nil }
func TestLastAnimationSubscriptionCancelsDownload(t *testing.T) {
	for _, source := range []string{"subscription", "service", "expiry", "authorization", "watcher_failure", "session"} {
		t.Run(source, func(t *testing.T) {
			entered := make(chan struct{})
			var eventsMu sync.Mutex
			var cancellations []AnimationObservation
			var body *cancelledAnimationBody
			s, _, _, _ := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
				var reader io.ReadCloser
				switch {
				case r.URL.Host == "i.pximg.net":
					body = &cancelledAnimationBody{ctx: r.Context(), entered: entered}
					reader = body
				case strings.Contains(r.URL.Path, "ugoira"):
					reader = io.NopCloser(strings.NewReader(`{"ugoira_metadata":{"zip_urls":{"medium":"https://i.pximg.net/frames.zip"},"frames":[{"file":"0.png","delay":80}]}}`))
				default:
					reader = io.NopCloser(strings.NewReader(`{"illust":{"id":42,"title":"animation","type":"ugoira","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7,"name":"artist"}}}`))
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: reader}, nil
			}, 1<<20)
			s.reads.config.ObserveAnimation = func(_ context.Context, event AnimationObservation) {
				if event.Stage == "animation_generation" {
					eventsMu.Lock()
					defer eventsMu.Unlock()
					cancellations = append(cancellations, event)
				}
			}
			if source == "expiry" {
				s.reads.config.TTL = time.Second
			}
			require.NoError(t, s.ConfigureAnimations(t.Context(), 1<<20))
			t.Cleanup(func() {
				err := s.CloseAnimations()
				if source == "watcher_failure" {
					require.ErrorContains(t, err, "animation authorization watcher failed")
				} else {
					require.NoError(t, err)
				}
			})
			ctx := animationContext(t, s)
			duringSession, cancelSession := context.WithCancel(ctx)
			defer cancelSession()
			invalidated := make(chan struct{})
			open := s.reads.open
			s.reads.open = func(request context.Context, account int64) (Session, error) {
				if request == duringSession {
					cancelSession()
				}
				session, err := open(request, account)
				session.Invalidated = invalidated
				return session, err
			}
			first, err := s.PrepareAnimation(ctx, 72, 42, "gif")
			require.NoError(t, err)
			second, err := s.PrepareAnimation(ctx, 72, 42, "gif")
			require.NoError(t, err)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("download did not start")
			}
			_, err = s.AnimationSubscription(ctx, 73, first.Handle, false)
			require.Error(t, err, "foreign account cannot cancel the subscription")
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			_, err = s.AnimationSubscription(cancelled, 72, first.Handle, false)
			require.Error(t, err)
			_, err = s.AnimationSubscription(duringSession, 72, first.Handle, false)
			require.Error(t, err)
			status, err := s.AnimationSubscription(ctx, 72, first.Handle, false)
			require.NoError(t, err)
			require.Equal(t, "downloading", status.State, "cancelled control requests must leave subscribed work running")
			_, err = s.AnimationSubscription(ctx, 72, first.Handle, true)
			require.NoError(t, err)
			require.False(t, body.closed.Load())
			switch source {
			case "subscription":
				_, err = s.AnimationSubscription(ctx, 72, second.Handle, true)
				require.NoError(t, err)
			case "service":
				require.NoError(t, s.CloseAnimations())
			case "authorization":
				_, err := s.store.Init(t.Context(), true)
				require.NoError(t, err)
				// 使用真实原子替换路径；撤销后不调用status或媒体读取触发检查。
			case "watcher_failure":
				require.NoError(t, os.Rename(filepath.Dir(s.store.Path), filepath.Join(t.TempDir(), "moved")))
			case "session":
				close(invalidated)
			}
			require.Eventually(t, body.closed.Load, 3*time.Second, time.Millisecond)
			require.Eventually(t, func() bool {
				files, err := os.ReadDir(s.animations.root)
				if source == "service" {
					return os.IsNotExist(err)
				}
				return err == nil && len(files) == 2 && files[0].Name() == "instance.lock" && files[1].Name() == "owner"
			}, time.Second, time.Millisecond)
			require.Eventually(t, func() bool {
				eventsMu.Lock()
				defer eventsMu.Unlock()
				return len(cancellations) == 1
			}, time.Second, time.Millisecond)
			eventsMu.Lock()
			observed := append([]AnimationObservation(nil), cancellations...)
			eventsMu.Unlock()
			expectedSource := source
			if source == "watcher_failure" || source == "session" {
				expectedSource = "authorization"
			}
			require.Equal(t, expectedSource, observed[0].CancellationSource)
			require.Zero(t, observed[0].Bytes)
			require.GreaterOrEqual(t, observed[0].Duration, time.Duration(0))
		})
	}
}

func TestAnimationObservesBytesAndReuseWithoutIdentifiers(t *testing.T) {
	s, ctx, _, _, _ := animationProxy(t, 1<<20)
	var mu sync.Mutex
	var events []AnimationObservation
	s.reads.config.ObserveAnimation = func(_ context.Context, event AnimationObservation) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}
	prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	ready := awaitAnimation(t, s, ctx, prepared.Handle)
	require.Equal(t, "ready", ready.State)
	_, err = s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	s.animations.wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	stages := map[string]AnimationObservation{}
	for _, event := range events {
		stages[event.Stage] = event
	}
	require.Contains(t, stages, "animation_zip_download")
	require.Contains(t, stages, "animation_encode_wait")
	require.Contains(t, stages, "animation_encode")
	require.Equal(t, ready.Size, stages["animation_ready"].Bytes)
	require.Contains(t, stages, "animation_generation")
	require.Equal(t, ready.Size, stages["animation_generation"].Bytes)
	for _, event := range stages {
		require.False(t, event.Failed, "successful phase: %s", event.Stage)
	}
	require.Contains(t, stages, "animation_cache_hit")
}

func TestConcurrentAnimationFormatsShareDiskWithoutPrematureCapacityFailure(t *testing.T) {
	s, ctx, _, _, _ := animationProxy(t, 1<<20)
	inputs := []string{"gif", "apng"}
	handles := make([]string, 2)
	var wg sync.WaitGroup
	for i, format := range inputs {
		wg.Add(1)
		go func(i int, format string) {
			defer wg.Done()
			prepared, err := s.PrepareAnimation(ctx, 72, 42, format)
			if err != nil {
				t.Error(err)
				return
			}
			handles[i] = prepared.Handle
		}(i, format)
	}
	wg.Wait()
	for _, handle := range handles {
		require.NotEmpty(t, handle)
		status := awaitAnimation(t, s, ctx, handle)
		require.Equal(t, "ready", status.State, status.Error)
	}
}

type interruptedAnimationWriter struct{ *httptest.ResponseRecorder }

type stalledAnimationWriter struct {
	*httptest.ResponseRecorder
	entered  chan struct{}
	released chan struct{}
	once     sync.Once
}

func (w *stalledAnimationWriter) Write([]byte) (int, error) {
	close(w.entered)
	<-w.released
	return 0, io.ErrClosedPipe
}

func (w *stalledAnimationWriter) SetWriteDeadline(deadline time.Time) error {
	if !deadline.After(time.Now()) {
		w.once.Do(func() { close(w.released) })
	}
	return nil
}

func TestAnimationCloseUnblocksPinnedTransfer(t *testing.T) {
	s, ctx, _, _, _ := animationProxy(t, 1<<20)
	observed := make(chan AnimationObservation, 1)
	s.reads.config.ObserveAnimation = func(_ context.Context, event AnimationObservation) {
		if event.Stage == "animation_delivery" {
			observed <- event
		}
	}
	prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	ready := awaitAnimation(t, s, ctx, prepared.Handle)
	require.Equal(t, "ready", ready.State)
	w := &stalledAnimationWriter{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), released: make(chan struct{})}
	delivered := make(chan any, 1)
	go func() {
		defer func() { delivered <- recover() }()
		s.ServeHTTP(w, httptest.NewRequest("GET", ready.AnimationURL, nil))
	}()
	defer func() {
		w.once.Do(func() { close(w.released) })
		require.Equal(t, http.ErrAbortHandler, <-delivered)
	}()
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("animation transfer did not enter the response writer")
	}
	closed := make(chan error, 1)
	go func() { closed <- s.CloseAnimations() }()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(time.Second):
		// 测试收尾解除阻塞，避免失败用例遗留服务关闭 goroutine。
		w.once.Do(func() { close(w.released) })
		require.NoError(t, <-closed)
		t.Fatal("service close remained blocked by a pinned animation transfer")
	}
	require.Equal(t, http.StatusOK, w.Code)
	event := <-observed
	require.Equal(t, "service", event.CancellationSource)
	require.Zero(t, event.Bytes)
}

func TestAnimationRevocationUnblocksPinnedTransfer(t *testing.T) {
	s, ctx, _, _, store := animationProxy(t, 1<<20)
	prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	ready := awaitAnimation(t, s, ctx, prepared.Handle)
	require.Equal(t, "ready", ready.State)
	w := &stalledAnimationWriter{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), released: make(chan struct{})}
	delivered := make(chan any, 1)
	go func() {
		defer func() { delivered <- recover() }()
		s.ServeHTTP(w, httptest.NewRequest("GET", ready.AnimationURL, nil))
	}()
	defer w.once.Do(func() { close(w.released) })
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("animation transfer did not enter the response writer")
	}
	_, err = store.Init(t.Context(), true)
	require.NoError(t, err)
	select {
	case result := <-delivered:
		require.Equal(t, http.ErrAbortHandler, result)
	case <-time.After(3 * time.Second):
		w.once.Do(func() { close(w.released) })
		<-delivered
		t.Fatal("revoked animation transfer remained blocked")
	}
	require.NoError(t, s.CloseAnimations())
}

func TestCancelledAnimationRequestPreservesSubscribedArtifact(t *testing.T) {
	s, ctx, _, _, _ := animationProxy(t, 1<<20)
	prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	ready := awaitAnimation(t, s, ctx, prepared.Handle)
	require.Equal(t, "ready", ready.State)
	requestCtx, cancel := context.WithCancel(t.Context())
	cancel()
	s.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", ready.AnimationURL, nil).WithContext(requestCtx))
	status, err := s.AnimationSubscription(ctx, 72, prepared.Handle, false)
	require.NoError(t, err)
	require.Equal(t, "ready", status.State, "cancelling one transfer must not expire independent subscribers")
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest("GET", ready.AnimationURL, nil))
	require.Equal(t, http.StatusOK, response.Code)
	frames, err := gif.DecodeAll(bytes.NewReader(response.Body.Bytes()))
	require.NoError(t, err)
	require.Len(t, frames.Image, 2)
}

func (w interruptedAnimationWriter) Write(data []byte) (int, error) {
	return 0, io.ErrClosedPipe
}
func TestAnimationDeliveryReportsActualInterruptedBytes(t *testing.T) {
	s, ctx, _, _, _ := animationProxy(t, 1<<20)
	var delivered AnimationObservation
	s.reads.config.ObserveAnimation = func(_ context.Context, event AnimationObservation) {
		if event.Stage == "animation_delivery" {
			delivered = event
		}
	}
	prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	ready := awaitAnimation(t, s, ctx, prepared.Handle)
	require.Equal(t, "ready", ready.State)
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		s.ServeHTTP(interruptedAnimationWriter{httptest.NewRecorder()}, httptest.NewRequest("GET", ready.AnimationURL, nil))
	})
	require.Zero(t, delivered.Bytes)
	require.Equal(t, "transport", delivered.CancellationSource)
}

func TestAnimationWorkspaceCleanupRequiresReleasedInstanceLock(t *testing.T) {
	parent := t.TempDir()
	root, release, err := newAnimationWorkspace(parent)
	require.NoError(t, err)
	unknown, err := os.MkdirTemp(parent, "pixiv-mcp-animation-")
	require.NoError(t, err)
	require.NoError(t, cleanupAnimationWorkspaces(parent))
	_, err = os.Stat(root)
	require.NoError(t, err, "live instance must survive")
	require.NoError(t, release())
	require.NoError(t, cleanupAnimationWorkspaces(parent))
	_, err = os.Stat(root)
	require.True(t, os.IsNotExist(err), "ended instance must be removed")
	_, err = os.Stat(unknown)
	require.NoError(t, err, "unknown directory must survive")
}

func animationFileNames(files []os.DirEntry) []string {
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = file.Name()
	}
	return names
}

func TestQueuedAnimationRevalidatesGrantBeforeContentRead(t *testing.T) {
	s, ctx, count, _, store := animationProxy(t, 1<<20)
	s.animations.buildGate <- struct{}{}
	prepared, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	state, err := store.Read(t.Context())
	require.NoError(t, err)
	s.animations.mu.Lock()
	job := s.animations.handles[prepared.Handle]
	s.animations.mu.Unlock()
	grant := state.Grants["g"]
	grant.Revoked = true
	state.Grants["g"] = grant
	body, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.Path, body, 0600))
	<-s.animations.buildGate
	require.Eventually(t, func() bool { s.animations.mu.Lock(); defer s.animations.mu.Unlock(); return job.done }, time.Second, time.Millisecond)
	require.Zero(t, count.Load(), "revoked queued work must not download frames")
}

func TestAnimationNativeReservationReclaimsUnusedReadyArtifact(t *testing.T) {
	s, ctx, _, _, _ := animationProxy(t, 1<<20)
	first, err := s.PrepareAnimation(ctx, 72, 42, "apng")
	require.NoError(t, err)
	require.Equal(t, "ready", awaitAnimation(t, s, ctx, first.Handle).State)
	_, err = s.AnimationSubscription(ctx, 72, first.Handle, true)
	require.NoError(t, err)
	s.animations.mu.Lock()
	s.animations.capacity = s.animations.used + int64(len(animationArchive(t))) + 16
	s.animations.mu.Unlock()
	next, err := s.PrepareAnimation(ctx, 72, 42, "gif")
	require.NoError(t, err)
	status := awaitAnimation(t, s, ctx, next.Handle)
	require.Equal(t, "ready", status.State, status.Error)
}

func TestAnimationEncodingPreservesOtherReadyFormatsWhenCapacityIsAvailable(t *testing.T) {
	s, ctx, downloads, _, _ := animationProxy(t, 1<<20)
	for _, format := range []string{"gif", "apng"} {
		prepared, err := s.PrepareAnimation(ctx, 72, 42, format)
		require.NoError(t, err)
		ready := awaitAnimation(t, s, ctx, prepared.Handle)
		require.Equal(t, "ready", ready.State, ready.Error)
		_, err = s.AnimationSubscription(ctx, 72, prepared.Handle, true)
		require.NoError(t, err)
	}
	for _, format := range []string{"gif", "apng"} {
		repeat, err := s.PrepareAnimation(ctx, 72, 42, format)
		require.NoError(t, err)
		require.Equal(t, "ready", repeat.State, "encoding another format must not evict a retained artifact while capacity is available")
		get := httptest.NewRecorder()
		s.ServeHTTP(get, httptest.NewRequest("GET", repeat.AnimationURL, nil))
		require.Equal(t, http.StatusOK, get.Code)
	}
	require.EqualValues(t, 2, downloads.Load(), "repeated preparation must not download archives again")
}
