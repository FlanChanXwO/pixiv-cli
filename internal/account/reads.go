package account

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
)

// ReadSession 持有短期 SDK client；凭据不会离开账号 owner。
type ReadSession struct {
	Client      *pixiv.Client
	Revision    int64
	Expires     time.Time
	Version     uint64
	Invalidated <-chan struct{}
}

// ReadSessions 单独管理 MCP 读取 client，与变更用的账号快照分开。
type ReadSessions struct {
	ctx        context.Context
	cancel     context.CancelFunc
	store      *Store
	gate       *Gate
	mu         sync.Mutex
	sessions   map[int64]readEntry
	generation uint64
	closed     bool
}
type readEntry struct {
	session   ReadSession
	network   string
	transport *http.Client
	cancel    context.CancelFunc
}

// NewReadSessions 与写路径共用已有的轮换门。
func NewReadSessions(store *Store, gate *Gate) *ReadSessions {
	ctx, cancel := context.WithCancel(context.Background())
	return &ReadSessions{ctx: ctx, cancel: cancel, store: store, gate: gate, sessions: map[int64]readEntry{}}
}

func (m *ReadSessions) cached(ctx context.Context, uid int64, network string) (ReadSession, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ReadSession{}, false, errors.New("read sessions closed")
	}
	a, err := m.store.GetPixiv(ctx, uid)
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

// Open 在成功持久化轮换后的凭据后返回共享读取快照。轮换门在返回前释放，内容与媒体请求
// 不持有它。
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
	err := m.gate.Acquire(ctx)
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
	session, err := m.store.openReadSession(ctx, uid, options)
	if err != nil {
		cancelGeneration()
		options.HTTPClient.CloseIdleConnections()
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
	// 其他进程可在轮换后再次提交凭据；发布缓存前复核 revision。
	account, err := m.store.GetPixiv(ctx, uid)
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
	transport.revision.Store(session.Revision)
	m.sessions[uid] = readEntry{session, network, options.HTTPClient, cancelGeneration}
	return session, nil
}

// Invalidate 只丢弃失败的 代次，保留并发替换产生的新值。
func (m *ReadSessions) Invalidate(uid int64, version uint64, err error) {
	var classified *sdk.Error
	if !errors.As(err, &classified) || (classified.Reason != sdk.Unauthorized && classified.Reason != sdk.CredentialsExpired && classified.HTTPStatus != 401) {
		return
	}
	m.discard(uid, version)
}

func (m *ReadSessions) discard(uid int64, version uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.sessions[uid]; ok && entry.session.Version == version {
		entry.close()
		delete(m.sessions, uid)
	}
}

// Close 在 MCP 宿主 生命周期结束时释放空闲传输。
func (m *ReadSessions) Close() error {
	// 先唤醒占有mu的本地锁等待与在途读取，不能先取得mu再取消生命周期。
	m.cancel()
	m.mu.Lock()
	m.closed = true
	for uid, entry := range m.sessions {
		entry.close()
		delete(m.sessions, uid)
	}
	m.mu.Unlock()
	return nil
}

func (e readEntry) close() {
	e.cancel()
	e.session.Client.CloseIdleConnections()
	e.transport.CloseIdleConnections()
}

// 原始读取租约（例如媒体 tool）在上游 401 时同样作废当前 代次，不重试失败请求，
// 也不改变 SDK 的错误分类。
type readSessionTransport struct {
	inner    http.RoundTripper
	owner    *ReadSessions
	userID   int64
	version  atomic.Uint64
	revision atomic.Int64
	ctx      context.Context
}

func (t *readSessionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	// 发布前的 OAuth 请求尚无 revision；发布后的每次内容请求均复核本地凭据。
	if revision := t.revision.Load(); revision > 0 {
		a, err := t.owner.store.GetPixiv(ctx, t.userID)
		if err == nil && a.CredentialRevision != revision {
			err = ErrCredentialConflict
		}
		if err != nil {
			stop()
			cancel()
			if errors.Is(err, ErrCredentialConflict) || errors.Is(err, ErrPixivNotFound) {
				t.owner.discard(t.userID, t.version.Load())
			}
			return nil, err
		}
	}
	response, err := t.inner.RoundTrip(r.Clone(ctx))
	if response != nil && response.StatusCode == http.StatusUnauthorized {
		// 返回本次401 完整正文；失效只主动取消同一代次的其他传输。
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
