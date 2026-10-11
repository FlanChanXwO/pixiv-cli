package mediaproxy

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/stretchr/testify/require"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCapabilityLifetimeAndBinding(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	state, err := store.Read(t.Context())
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("fixture"))
	hash := hex.EncodeToString(sum[:])
	expiry := time.Now().Add(time.Hour)
	state.Clients["c"] = auth.Client{RedirectURIs: []string{"https://client.test/callback"}}
	state.Grants["g"] = auth.Grant{ClientID: "c", Resource: "https://example.test/mcp", Scope: "mcp", AccessTokens: map[string]time.Time{hash: expiry}, RefreshHash: strings.Repeat("a", 64), UsedRefreshHashes: []string{}}
	save := func() {
		b, e := json.Marshal(state)
		require.NoError(t, e)
		require.NoError(t, os.WriteFile(store.Path, b, 0600))
	}
	save()
	service, err := New("https://example.test", store, nil)
	require.NoError(t, err)
	h, err := auth.NewHandler("https://example.test", store)
	require.NoError(t, err)
	var capability string
	issue := h.RequireBearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := sdkauth.TokenInfoFromContext(r.Context())
		require.NotNil(t, info)
		var e error
		capability, e = service.Issue(r.Context(), 72, 42, 1, "thumbnail")
		require.NoError(t, e)
	}))
	req := httptest.NewRequest("POST", "https://example.test/mcp", nil)
	req.Header.Set("Authorization", "Bearer fixture")
	issue.ServeHTTP(httptest.NewRecorder(), req)
	require.NotEmpty(t, capability)
	require.NotContains(t, capability, "fixture")
	claims, err := service.verify(context.Background(), strings.TrimPrefix(capability, "https://example.test/media/"))
	require.NoError(t, err)
	require.EqualValues(t, 72, claims.Account)
	require.EqualValues(t, 42, claims.Artwork)
	require.Equal(t, "thumbnail", claims.Quality)
	_, err = service.verify(t.Context(), strings.TrimPrefix(capability, "https://example.test/media/")+"x")
	require.Error(t, err)
	restarted, err := New("https://example.test", store, nil)
	require.NoError(t, err)
	_, err = restarted.verify(t.Context(), strings.TrimPrefix(capability, "https://example.test/media/"))
	require.Error(t, err)

	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	var closes, reads int
	badImage := false
	badHost := false
	service.open = func(ctx context.Context, uid int64) (*lifecycle.Lease[*pixiv.Client], error) {
		require.EqualValues(t, 72, uid)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		client, e := pixiv.NewWith("fixture-upstream-token", pixiv.Options{HTTPClient: &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "app-api.pixiv.net" {
				imageURL := "https://i.pximg.net/img-original/img/2026/01/01/00/00/00/42_p0.png"
				if badHost {
					imageURL = "http://127.0.0.1/private"
				}
				b, _ := json.Marshal(map[string]any{"illust": map[string]any{"id": 42, "title": "fixture", "type": "illust", "page_count": 1, "create_date": "2026-01-01T00:00:00Z", "user": map[string]any{"id": 7}, "meta_single_page": map[string]any{"original_image_url": imageURL}}})
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(b))}, nil
			}
			require.Equal(t, "i.pximg.net", r.URL.Host)
			require.Empty(t, r.Header.Get("Authorization"))
			require.Empty(t, r.Header.Get("Cookie"))
			require.NotEmpty(t, r.Header.Get("Referer"))
			reads++
			data := pngBytes.Bytes()
			if badImage {
				data = []byte("<svg>fixture-secret</svg>")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
		})}})
		return lifecycle.NewLease(client, func() error { closes++; return nil }), e
	}
	// Changing the shared selection must not retarget an already-issued image capability.
	state.SelectedPixivUserID = 99
	save()
	for _, method := range []string{"GET", "HEAD"} {
		w := httptest.NewRecorder()
		service.ServeHTTP(w, httptest.NewRequest(method, capability, nil))
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, "image/png", w.Header().Get("Content-Type"))
		require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
		if method == "GET" {
			require.Equal(t, pngBytes.Bytes(), w.Body.Bytes())
		} else {
			require.Empty(t, w.Body.Bytes())
		}
	}
	require.Equal(t, 2, reads)
	require.Equal(t, 2, closes)
	badImage = true
	w := httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest("GET", capability, nil))
	require.Equal(t, 502, w.Code)
	require.NotContains(t, w.Body.String(), "fixture-secret")
	badImage = false
	badHost = true
	w = httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest("GET", capability, nil))
	require.Equal(t, 502, w.Code)
	require.Equal(t, 3, reads, "unsafe resource must never reach the image transport")
	badHost = false
	for _, method := range []string{"POST", "DELETE"} {
		w := httptest.NewRecorder()
		service.ServeHTTP(w, httptest.NewRequest(method, capability, nil))
		require.Equal(t, 405, w.Code)
	}
	w = httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest("GET", capability+"?url=http://127.0.0.1", nil))
	require.Equal(t, 400, w.Code)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	w = httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest("GET", capability, nil).WithContext(canceled))
	require.Equal(t, 401, w.Code)
	expired := claims
	expired.Expires = time.Now().Add(-time.Second)
	require.Error(t, service.authorized(t.Context(), expired))
	service.open = func(context.Context, int64) (*lifecycle.Lease[*pixiv.Client], error) {
		return lifecycle.NewLease[*pixiv.Client](nil, func() error { closes++; return nil }), nil
	}
	beforeClose := closes
	w = httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest("GET", capability, nil))
	require.Equal(t, 502, w.Code)
	require.Equal(t, beforeClose+1, closes, "invalid client lease must still close")
	g := state.Grants["g"]
	g.Revoked = true
	state.Grants["g"] = g
	save()
	_, err = service.verify(t.Context(), strings.TrimPrefix(capability, "https://example.test/media/"))
	require.Error(t, err)
}

func TestImageStreamsBeforeUpstreamEOF(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	state, err := store.Read(t.Context())
	require.NoError(t, err)
	state.Clients["c"] = auth.Client{RedirectURIs: []string{"https://client.test/cb"}}
	state.Grants["g"] = auth.Grant{ClientID: "c", Resource: "https://example.test/mcp", Scope: "mcp", AccessTokens: map[string]time.Time{}, RefreshHash: strings.Repeat("a", 64), UsedRefreshHashes: []string{}}
	b, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.Path, b, 0600))
	upstream, send := io.Pipe()
	defer upstream.Close()
	defer send.Close()
	client, err := pixiv.NewWith("fixture", pixiv.Options{HTTPClient: &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "app-api.pixiv.net" {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"illust":{"id":42,"title":"fixture","type":"illust","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7},"meta_single_page":{"original_image_url":"https://i.pximg.net/img-original/img/2026/01/01/00/00/00/42_p0.png"}}}`))}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: upstream}, nil
	})}})
	require.NoError(t, err)
	service, err := New("https://example.test", store, func(context.Context, int64) (*lifecycle.Lease[*pixiv.Client], error) {
		return lifecycle.NewLease(client, nil), nil
	})
	require.NoError(t, err)
	c := claims{"g", 72, 42, 1, "thumbnail", time.Now().Add(time.Hour)}
	payload, _ := json.Marshal(c)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, service.key[:])
	mac.Write([]byte(encoded))
	target := "https://example.test/media/" + encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	downstream := httptest.NewServer(service)
	defer downstream.Close()
	responseCh := make(chan *http.Response, 1)
	errorCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		request, e := http.NewRequestWithContext(ctx, "GET", strings.Replace(target, "https://example.test", downstream.URL, 1), nil)
		if e != nil {
			errorCh <- e
			return
		}
		response, e := downstream.Client().Do(request)
		if e != nil {
			errorCh <- e
			return
		}
		responseCh <- response
	}()
	prefix := make([]byte, 512)
	copy(prefix, []byte("\x89PNG\r\n\x1a\n"))
	go func() { _, _ = send.Write(prefix) }()
	var response *http.Response
	select {
	case response = <-responseCh:
	case e := <-errorCh:
		t.Fatal(e)
	case <-time.After(2 * time.Second):
		cancel()
		send.Close()
		t.Fatal("downstream headers waited for upstream EOF")
	}
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	require.Empty(t, response.Header.Get("Content-Length"))
	first := make([]byte, len(prefix))
	_, err = io.ReadFull(response.Body, first)
	require.NoError(t, err)
	require.Equal(t, prefix, first)
	// Reading these bytes completes while the upstream pipe is still open.
	require.NoError(t, send.Close())
	rest, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Empty(t, rest)

}

func newCachedProxyFixture(t *testing.T, rt fixtureTransport, capacity int64) (*Service, string, *atomic.Uint64, auth.Store) {
	t.Helper()
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	state, err := store.Read(t.Context())
	require.NoError(t, err)
	state.Clients["c"] = auth.Client{RedirectURIs: []string{"https://client.test/cb"}}
	state.Grants["g"] = auth.Grant{ClientID: "c", Resource: "https://example.test/mcp", Scope: "mcp", AccessTokens: map[string]time.Time{}, RefreshHash: strings.Repeat("a", 64), UsedRefreshHashes: []string{}}
	b, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.Path, b, 0600))
	client, err := pixiv.NewWith("fixture", pixiv.Options{HTTPClient: &http.Client{Transport: rt}})
	require.NoError(t, err)
	service, err := New("https://example.test", store, nil)
	require.NoError(t, err)
	version := &atomic.Uint64{}
	version.Store(1)
	require.NoError(t, service.Configure(Config{Capacity: capacity, TTL: time.Minute, Concurrency: 2}, func(ctx context.Context, uid int64) (Session, error) {
		return Session{Lease: lifecycle.NewLease(client, nil), Version: fmt.Sprint(version.Load()), Expires: time.Now().Add(time.Hour)}, ctx.Err()
	}))
	c := claims{"g", 72, 42, 1, "thumbnail", time.Now().Add(time.Hour)}
	return service, signFixture(service, c), version, store
}
func TestReadPhaseFailuresKeepTheirOutcome(t *testing.T) {
	for _, stage := range []string{"session_acquire", "artwork_query"} {
		t.Run(stage, func(t *testing.T) {
			s, _, _, _ := newCachedProxyFixture(t, func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"fixture-private-error"}`))}, nil
			}, 1<<20)
			if stage == "session_acquire" {
				s.reads.open = func(context.Context, int64) (Session, error) {
					return Session{}, fmt.Errorf("fixture-private-session-error")
				}
			}
			var events []AnimationObservation
			s.reads.config.Observe = func(_ context.Context, stage string, duration time.Duration) {
				events = append(events, AnimationObservation{Stage: stage, Duration: duration})
			}
			s.reads.config.ObserveAnimation = func(_ context.Context, event AnimationObservation) { events = append(events, event) }
			_, err := s.Artwork(animationContext(t, s), 72, 42)
			require.Error(t, err)
			var matching []AnimationObservation
			for _, event := range events {
				if event.Stage == stage {
					matching = append(matching, event)
				}
			}
			require.Len(t, matching, 1)
			require.True(t, matching[0].Failed)
			require.Positive(t, matching[0].Duration)
			require.NotContains(t, fmt.Sprint(events), "fixture-private")
		})
	}
}

func signFixture(s *Service, c claims) string {
	payload, _ := json.Marshal(c)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte(encoded))
	return "https://example.test/media/" + encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func fixtureArtwork(id string) *http.Response {
	body := fmt.Sprintf(`{"illust":{"id":%s,"title":"art","type":"illust","page_count":1,"create_date":"2026-01-01T00:00:00Z","user":{"id":7},"meta_single_page":{"original_image_url":"https://i.pximg.net/img-original/img/2026/01/01/00/00/00/%s_p0.png"}}}`, id, id)
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestImageCacheReauthorizesAndSeparatesGenerations(t *testing.T) {
	var queries, images atomic.Int64
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	service, target, version, store := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "app-api.pixiv.net" {
			queries.Add(1)
			return fixtureArtwork(r.URL.Query().Get("illust_id")), nil
		}
		images.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Length": {strconv.Itoa(data.Len())}}, Body: io.NopCloser(bytes.NewReader(data.Bytes()))}, nil
	}, 65536)
	get := func(method string, target string) int {
		w := httptest.NewRecorder()
		service.ServeHTTP(w, httptest.NewRequest(method, target, nil))
		if w.Code == 200 && method == "GET" {
			require.Equal(t, data.Bytes(), w.Body.Bytes())
		}
		return w.Code
	}
	require.Equal(t, 200, get("GET", target))
	require.Equal(t, 200, get("GET", target))
	require.Equal(t, 200, get("HEAD", target))
	require.EqualValues(t, 1, queries.Load())
	require.EqualValues(t, 1, images.Load())
	version.Add(1)
	require.Equal(t, 200, get("GET", target))
	require.EqualValues(t, 2, queries.Load())
	require.EqualValues(t, 2, images.Load())
	for _, c := range []claims{{"g", 73, 42, 1, "thumbnail", time.Now().Add(time.Hour)}, {"g", 72, 42, 1, "regular", time.Now().Add(time.Hour)}} {
		require.Equal(t, 200, get("GET", signFixture(service, c)))
	}
	require.EqualValues(t, 3, queries.Load())
	require.EqualValues(t, 4, images.Load())
	state, err := store.Read(t.Context())
	require.NoError(t, err)
	g := state.Grants["g"]
	g.Revoked = true
	state.Grants["g"] = g
	body, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.Path, body, 0600))
	require.Equal(t, 401, get("GET", target))
	require.EqualValues(t, 4, images.Load())
}
func TestAuthorizationRevocationReclaimsIdleStaticCache(t *testing.T) {
	service, target, _, store := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "app-api.pixiv.net" {
			return fixtureArtwork("42"), nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader([]byte("\x89PNG\r\n\x1a\nfixture")))}, nil
	}, 8192)
	require.NoError(t, service.ConfigureAnimations(t.Context(), 1<<20))
	t.Cleanup(func() { require.NoError(t, service.CloseAnimations()) })
	w := httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
	require.Equal(t, http.StatusOK, w.Code)
	cache := service.reads.cache
	cache.mu.Lock()
	used := cache.used
	cache.mu.Unlock()
	require.Positive(t, used)
	_, err := store.Init(t.Context(), true)
	require.NoError(t, err)
	// 撤销后不读取旧媒体触发懒惰淘汰，通知本身必须释放空闲预算。
	require.Eventually(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return cache.used == 0
	}, 3*time.Second, time.Millisecond)
}

func TestSessionInvalidationReclaimsIdleStaticCache(t *testing.T) {
	service, target, _, _ := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "app-api.pixiv.net" {
			return fixtureArtwork("42"), nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader([]byte("\x89PNG\r\n\x1a\nfixture")))}, nil
	}, 8192)
	invalidated := make(chan struct{})
	open := service.reads.open
	require.NoError(t, service.Configure(service.reads.config, func(ctx context.Context, account int64) (Session, error) {
		session, err := open(ctx, account)
		session.Invalidated = invalidated
		return session, err
	}))
	require.NoError(t, service.ConfigureAnimations(t.Context(), 1<<20))
	t.Cleanup(func() { require.NoError(t, service.CloseAnimations()) })
	w := httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
	require.Equal(t, http.StatusOK, w.Code)
	cache := service.reads.cache
	cache.mu.Lock()
	used := cache.used
	cache.mu.Unlock()
	require.Positive(t, used)
	close(invalidated)
	require.Eventually(t, func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return cache.used == 0
	}, time.Second, time.Millisecond)
}

func TestInvalidatedSessionCannotRepublishCompletedImage(t *testing.T) {
	invalidated := make(chan struct{})
	release := make(chan struct{})
	close(release)
	prefix := make([]byte, 512)
	copy(prefix, []byte("\x89PNG\r\n\x1a\n"))
	var service *Service
	var target string
	service, target, _, _ = newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "app-api.pixiv.net" {
			return fixtureArtwork("42"), nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &controlledImageBody{prefix: append([]byte(nil), prefix...), release: release, ctx: r.Context(), close: func() {
			close(invalidated)
			// 先确认失效通知已清理旧条目，再让完整读取提交缓存。
			require.Eventually(t, func() bool {
				service.reads.cache.mu.Lock()
				defer service.reads.cache.mu.Unlock()
				return len(service.reads.cache.entries) == 0
			}, time.Second, time.Millisecond)
		}}}, nil
	}, 8192)
	open := service.reads.open
	require.NoError(t, service.Configure(service.reads.config, func(ctx context.Context, account int64) (Session, error) {
		session, err := open(ctx, account)
		session.Invalidated = invalidated
		return session, err
	}))
	require.NoError(t, service.ConfigureAnimations(t.Context(), 1<<20))
	t.Cleanup(func() { require.NoError(t, service.CloseAnimations()) })
	w := httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, prefix, w.Body.Bytes())
	service.reads.cache.mu.Lock()
	defer service.reads.cache.mu.Unlock()
	require.Zero(t, service.reads.cache.used, "completed reads cannot repopulate an invalidated credential generation")
}

func TestImageCacheNeverStoresInterruptedOrOversizedImages(t *testing.T) {
	for _, capacity := range []int64{0, 32, 8192} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			var calls atomic.Int64
			broken := true
			prefix := make([]byte, 512)
			copy(prefix, []byte("\x89PNG\r\n\x1a\n"))
			service, target, _, _ := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "app-api.pixiv.net" {
					return fixtureArtwork("42"), nil
				}
				calls.Add(1)
				size := len(prefix)
				if broken {
					size++
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Length": {strconv.Itoa(size)}}, Body: io.NopCloser(bytes.NewReader(prefix))}, nil
			}, capacity)
			require.PanicsWithValue(t, http.ErrAbortHandler, func() { service.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", target, nil)) })
			broken = false
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				service.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
				require.Equal(t, 200, w.Code)
				require.Equal(t, prefix, w.Body.Bytes())
			}
			want := int64(3)
			if capacity == 8192 {
				want = 2
			}
			require.Equal(t, want, calls.Load())
		})
	}
}

func TestMetadataCoalescesReadsAndBoundsConcurrency(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 20)
	var calls, active, peak atomic.Int64
	service, _, _, _ := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		current := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); current > old; old = peak.Load() {
			if peak.CompareAndSwap(old, current) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return fixtureArtwork(r.URL.Query().Get("illust_id")), nil
	}, 65536)
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	session, err := service.session(t.Context(), 72)
	require.NoError(t, err)
	defer session.Lease.Close()
	c := claims{"g", 72, 42, 1, "thumbnail", time.Now().Add(time.Hour)}
	first := make(chan error, 1)
	go func() { _, e := service.metadata(t.Context(), c, session); first <- e }()
	<-entered
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = service.metadata(canceled, c, session)
	require.Error(t, err)
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := service.metadata(t.Context(), c, session); failures <- e }()
	}
	close(release)
	require.NoError(t, <-first)
	wg.Wait()
	close(failures)
	for e := range failures {
		require.NoError(t, e)
	}
	require.EqualValues(t, 1, calls.Load())
	// Different keys still share the configured metadata semaphore.
	release = make(chan struct{})
	calls.Store(0)
	peak.Store(0)
	for i := int64(50); i < 58; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			claim := c
			claim.Artwork = id
			_, e := service.metadata(t.Context(), claim, session)
			if e != nil {
				t.Error(e)
			}
		}(i)
	}
	<-entered
	<-entered
	close(release)
	wg.Wait()
	require.EqualValues(t, 8, calls.Load())
	require.EqualValues(t, 2, peak.Load())
}

type controlledImageBody struct {
	prefix  []byte
	release <-chan struct{}
	ctx     context.Context
	close   func()
	once    sync.Once
}

func (b *controlledImageBody) Read(p []byte) (int, error) {
	if len(b.prefix) > 0 {
		n := copy(p, b.prefix)
		b.prefix = b.prefix[n:]
		return n, nil
	}
	select {
	case <-b.release:
		return 0, io.EOF
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	}
}
func (b *controlledImageBody) Close() error { b.once.Do(b.close); return nil }
func TestImageReadsHaveIndependentConcurrencyLimit(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 8)
	var active, peak, closes atomic.Int64
	service, target, _, _ := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "app-api.pixiv.net" {
			return fixtureArtwork("42"), nil
		}
		current := active.Add(1)
		for old := peak.Load(); current > old; old = peak.Load() {
			if peak.CompareAndSwap(old, current) {
				break
			}
		}
		entered <- struct{}{}
		prefix := make([]byte, 512)
		copy(prefix, []byte("\x89PNG\r\n\x1a\n"))
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &controlledImageBody{prefix: prefix, release: release, ctx: r.Context(), close: func() { active.Add(-1); closes.Add(1) }}}, nil
	}, 0)
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			service.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
			if w.Code != 200 {
				t.Errorf("status %d", w.Code)
			}
		}()
	}
	<-entered
	<-entered
	close(release)
	wg.Wait()
	require.EqualValues(t, 2, peak.Load())
	require.EqualValues(t, 8, closes.Load())
	require.Zero(t, active.Load())
}

type failedCloseBody struct{ io.Reader }

func (failedCloseBody) Close() error { return errors.New("fixture close failure") }
func TestFailedResourceCloseDoesNotPopulateImageCache(t *testing.T) {
	var calls atomic.Int64
	prefix := make([]byte, 512)
	copy(prefix, []byte("\x89PNG\r\n\x1a\n"))
	service, target, _, _ := newCachedProxyFixture(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "app-api.pixiv.net" {
			return fixtureArtwork("42"), nil
		}
		calls.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: failedCloseBody{bytes.NewReader(prefix)}}, nil
	}, 8192)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		service.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
		require.Equal(t, 200, w.Code)
		require.Equal(t, prefix, w.Body.Bytes())
	}
	require.EqualValues(t, 2, calls.Load())
}
