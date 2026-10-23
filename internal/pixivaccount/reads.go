package pixivaccount

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// ReadSession holds an ephemeral SDK client; credentials never leave the account owner.
type ReadSession struct {
	Client      *pixiv.Client
	Revision    int64
	Expires     time.Time
	Version     uint64
	Invalidated <-chan struct{}
}

// ReadSessions owns MCP read clients separately from mutation snapshots.
type ReadSessions struct {
	ctx     context.Context
	cancel  context.CancelFunc
	service *Service
	gate    interface {
		Acquire(context.Context) error
		Release()
	}
	mu          sync.Mutex
	sessions    map[int64]readEntry
	generation  uint64
	closed      bool
	watchCancel context.CancelFunc
	watchDone   chan struct{}
	watchErr    error
}
type readEntry struct {
	session   ReadSession
	network   string
	transport *http.Client
	cancel    context.CancelFunc
}

// NewReadSessions shares the existing rotation gate with writes.
func NewReadSessions(service *Service, gate interface {
	Acquire(context.Context) error
	Release()
}) *ReadSessions {
	ctx, cancel := context.WithCancel(context.Background())
	return &ReadSessions{ctx: ctx, cancel: cancel, service: service, gate: gate, sessions: map[int64]readEntry{}}
}

func (m *ReadSessions) cached(ctx context.Context, uid int64, network string) (ReadSession, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ReadSession{}, false, errors.New("read sessions closed")
	}
	a, err := m.service.repo.GetPixiv(ctx, uid)
	// 单次查询取消不代表凭据失效，不能连带终止共享传输或触发轮换。
	if cancelled := ctx.Err(); cancelled != nil {
		return ReadSession{}, false, cancelled
	}
	entry, ok := m.sessions[uid]
	if ok && err == nil && entry.network == network && entry.session.Revision == a.CredentialRevision && time.Now().Before(entry.session.Expires) {
		return entry.session, true, nil
	}
	if ok {
		entry.close()
		delete(m.sessions, uid)
	}
	return ReadSession{}, false, err
}

// Open returns a shared read snapshot after successfully persisting rotated credentials.
// The rotation gate is released before returning; content and media never own it.
func (m *ReadSessions) Open(ctx context.Context, uid int64, network string, options pixiv.Options) (ReadSession, error) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.ctx, cancel)
	defer func() { stop(); cancel() }()
	if err := ctx.Err(); err != nil {
		return ReadSession{}, err
	}
	if session, ok, err := m.cached(ctx, uid, network); ok || err != nil {
		return session, err
	}
	wait := time.Now()
	err := m.gate.Acquire(ctx)
	observeReadSession(ctx, "session_wait", wait, err)
	if err != nil {
		return ReadSession{}, err
	}
	defer m.gate.Release()
	if session, ok, err := m.cached(ctx, uid, network); ok || err != nil {
		return session, err
	}
	generationCtx, cancelGeneration := context.WithCancel(m.ctx)
	transport := &readSessionTransport{owner: m, userID: uid, ctx: generationCtx}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	}
	clientCopy := *options.HTTPClient
	transport.inner = clientCopy.Transport
	if transport.inner == nil {
		transport.inner = http.DefaultTransport
	}
	clientCopy.Transport = transport
	options.HTTPClient = &clientCopy
	started := time.Now()
	session, err := m.service.openReadSession(ctx, uid, options)
	observeReadSession(ctx, "session_refresh", started, err)
	if err != nil {
		cancelGeneration()
		if options.HTTPClient != nil {
			options.HTTPClient.CloseIdleConnections()
		}
		return ReadSession{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		cancelGeneration()
		options.HTTPClient.CloseIdleConnections()
		session.Client.CloseIdleConnections()
		return ReadSession{}, errors.New("read sessions closed")
	}
	// 文件事件可能已在轮换后、发布前被消费；发布时必须再次核对本地generation。
	account, err := m.service.repo.GetPixiv(ctx, uid)
	if err == nil && account.CredentialRevision != session.Revision {
		err = ErrCredentialConflict
	}
	if err != nil {
		cancelGeneration()
		options.HTTPClient.CloseIdleConnections()
		session.Client.CloseIdleConnections()
		return ReadSession{}, err
	}
	m.generation++
	session.Version = m.generation
	session.Invalidated = generationCtx.Done()
	transport.version.Store(session.Version)
	m.sessions[uid] = readEntry{session, network, options.HTTPClient, cancelGeneration}
	return session, nil
}

func observeReadSession(ctx context.Context, stage string, started time.Time, err error) {
	event := logging.Event{Module: logging.ModulePixivMCP, Kind: logging.EventCompleted, Operation: stage, Duration: time.Since(started)}
	if err != nil {
		event.Kind = logging.EventFailed
		event.Reason = logging.ReasonToolFailed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			event.Reason = logging.ReasonRequestCancelled
		}
	}
	logging.Emit(ctx, event)
}

// Revalidate checks local credential generations without opening an upstream session.
func (m *ReadSessions) Revalidate(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.ctx, cancel)
	defer func() { stop(); cancel() }()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("read sessions closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var failures error
	for uid, entry := range m.sessions {
		account, err := m.service.repo.GetPixiv(ctx, uid)
		// 监听生命周期取消不表示账号失效；由Close统一取消仍有效的会话。
		if cancelled := ctx.Err(); cancelled != nil {
			return errors.Join(failures, cancelled)
		}
		if err != nil && !errors.Is(err, ErrNotFound) && m.watchDone != nil {
			// 在唤醒失效订阅前发布watch失败，防止并发Close取消并掩盖已发生的存储错误。
			m.watchErr = errors.New("read session credential watcher failed")
			m.closed = true
		}
		if err != nil || account.CredentialRevision != entry.session.Revision || !time.Now().Before(entry.session.Expires) {
			entry.close()
			delete(m.sessions, uid)
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			failures = errors.Join(failures, err)
		}
	}
	return failures
}

// Invalidate discards only the failed generation, preserving a concurrent replacement.
func (m *ReadSessions) Invalidate(uid int64, version uint64, err error) {
	var classified *sdk.Error
	if !errors.As(err, &classified) || (classified.Reason != sdk.Unauthorized && classified.Reason != sdk.CredentialsExpired && classified.HTTPStatus != 401) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.sessions[uid]; ok && entry.session.Version == version {
		entry.close()
		delete(m.sessions, uid)
	}
}

// Close releases idle transports at the MCP owner lifecycle boundary.
func (m *ReadSessions) Close() error {
	// 先唤醒占有mu的本地锁等待与在途读取，不能先取得mu再取消生命周期。
	m.cancel()
	m.mu.Lock()
	m.closed = true
	if m.watchCancel != nil {
		m.watchCancel()
	}
	done := m.watchDone
	for uid, entry := range m.sessions {
		entry.close()
		delete(m.sessions, uid)
	}
	m.mu.Unlock()
	if done != nil {
		<-done
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.watchErr
}

func (e readEntry) close() {
	e.cancel()
	e.session.Client.CloseIdleConnections()
	if e.transport != nil {
		e.transport.CloseIdleConnections()
	}
}

// Raw read leases (for example media tools) also invalidate on upstream 401,
// without retrying the failed request or changing SDK error classification.
type readSessionTransport struct {
	inner   http.RoundTripper
	owner   *ReadSessions
	userID  int64
	version atomic.Uint64
	ctx     context.Context
}

func (t *readSessionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	response, err := t.inner.RoundTrip(r.Clone(ctx))
	if response != nil && response.StatusCode == http.StatusUnauthorized {
		// 返回本次401完整正文；失效只主动取消同generation的其他传输。
		stop()
		t.owner.Invalidate(t.userID, t.version.Load(), sdk.NewError("pixiv", "Read", sdk.Unauthorized))
	}
	if response == nil || response.Body == nil || err != nil {
		stop()
		cancel()
	} else {
		response.Body = &readSessionBody{ReadCloser: response.Body, stop: stop, cancel: cancel}
	}
	return response, err
}

type readSessionBody struct {
	io.ReadCloser
	stop   func() bool
	cancel context.CancelFunc
}

func (b *readSessionBody) Close() error {
	defer b.cancel()
	b.stop()
	return b.ReadCloser.Close()
}
func (t *readSessionTransport) CloseIdleConnections() {
	if closer, ok := t.inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
