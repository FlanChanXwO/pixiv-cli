package account

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk"
	legacypixiv "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// ReadSession 关联短期 SDK client 与已保存的凭据版本；Invalidated 通知该代次失效。
type ReadSession struct {
	Client      *legacypixiv.Client
	SDKClient   *pixiv.Client
	createdAt   time.Time
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
	sessions   map[readKey]readEntry
	generation uint64
	closed     bool
}
type readKey struct {
	userID int64
	sdk    bool
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
	return &ReadSessions{ctx: ctx, cancel: cancel, store: store, gate: gate, sessions: map[readKey]readEntry{}}
}

func (m *ReadSessions) cached(ctx context.Context, key readKey, network string) (ReadSession, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ReadSession{}, false, errors.New("read sessions closed")
	}
	a, err := m.store.GetPixiv(ctx, key.userID)
	// 单次查询取消不代表凭据失效，不能连带终止共享传输或触发轮换。
	if cancelled := ctx.Err(); cancelled != nil {
		return ReadSession{}, false, cancelled
	}
	entry, ok := m.sessions[key]
	if ok && err == nil && entry.network == network && entry.session.Revision == a.CredentialRevision && entry.session.createdAt.Equal(a.CreatedAt) && time.Now().Before(entry.session.Expires) {
		return entry.session, true, nil
	}
	if ok {
		entry.close()
		delete(m.sessions, key)
	}
	return ReadSession{}, false, err
}

// Open 在成功持久化轮换后的凭据后返回共享读取快照。轮换门在返回前释放，内容与媒体请求
// 不持有它。
func (m *ReadSessions) Open(ctx context.Context, uid int64, network string, options legacypixiv.Options) (ReadSession, error) {
	return m.open(ctx, readKey{userID: uid}, network, options.HTTPClient, func(ctx context.Context, httpClient *http.Client) (ReadSession, error) {
		options.HTTPClient = httpClient
		return m.store.openReadSession(ctx, uid, options)
	})
}

// OpenSDK 在轮换持久化完成后交出新 SDK 读取快照；内容请求不占用共享轮换门。
func (m *ReadSessions) OpenSDK(ctx context.Context, uid int64, network string, options pixiv.Options) (ReadSession, error) {
	return m.open(ctx, readKey{userID: uid, sdk: true}, network, options.HTTPClient, func(ctx context.Context, httpClient *http.Client) (ReadSession, error) {
		options.HTTPClient = httpClient
		account, err := m.store.GetPixiv(ctx, uid)
		if err != nil {
			return ReadSession{}, err
		}
		client, err := pixiv.New(readCredentials{savedCredentials: savedCredentials{store: m.store, userID: uid, createdAt: account.CreatedAt}, transport: httpClient.Transport.(*readSessionTransport)}, options)
		if err != nil {
			return ReadSession{}, err
		}
		credentials, err := client.Credentials(ctx)
		if err != nil {
			client.CloseIdleConnections()
			return ReadSession{}, err
		}
		return ReadSession{SDKClient: client, createdAt: account.CreatedAt, Revision: credentials.Revision, Expires: credentials.ExpiresAt}, nil
	})
}

func (m *ReadSessions) open(ctx context.Context, key readKey, network string, httpClient *http.Client, create func(context.Context, *http.Client) (ReadSession, error)) (ReadSession, error) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.ctx, cancel)
	defer func() { stop(); cancel() }()
	if err := ctx.Err(); err != nil {
		return ReadSession{}, err
	}
	if session, ok, err := m.cached(ctx, key, network); ok || err != nil {
		return session, err
	}
	err := m.gate.Acquire(ctx)
	if err != nil {
		return ReadSession{}, err
	}
	defer m.gate.Release()
	if session, ok, err := m.cached(ctx, key, network); ok || err != nil {
		return session, err
	}
	generationCtx, cancelGeneration := context.WithCancel(m.ctx)
	transport := &readSessionTransport{owner: m, userID: key.userID, ctx: generationCtx}
	if httpClient == nil {
		httpClient = &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	}
	clientCopy := *httpClient
	transport.inner = clientCopy.Transport
	if transport.inner == nil {
		transport.inner = http.DefaultTransport
	}
	clientCopy.Transport = transport
	httpClient = &clientCopy
	session, err := create(ctx, httpClient)
	if err != nil {
		cancelGeneration()
		httpClient.CloseIdleConnections()
		return ReadSession{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		cancelGeneration()
		httpClient.CloseIdleConnections()
		session.closeIdleConnections()
		return ReadSession{}, errors.New("read sessions closed")
	}
	// 其他进程可在轮换后再次提交凭据；发布缓存前复核 revision。
	account, err := m.store.GetPixiv(ctx, key.userID)
	if err == nil && (account.CredentialRevision != session.Revision || !account.CreatedAt.Equal(session.createdAt)) {
		err = ErrCredentialConflict
	}
	if err != nil {
		cancelGeneration()
		httpClient.CloseIdleConnections()
		session.closeIdleConnections()
		return ReadSession{}, err
	}
	m.generation++
	session.Version = m.generation
	session.Invalidated = generationCtx.Done()
	transport.version.Store(session.Version)
	transport.createdAt = session.createdAt
	transport.revision.Store(session.Revision)
	m.sessions[key] = readEntry{session, network, httpClient, cancelGeneration}
	return session, nil
}

// Invalidate 只丢弃失败的代次，保留并发替换产生的新值。
func (m *ReadSessions) Invalidate(uid int64, version uint64, err error) {
	var classified *sdk.Error
	var previous *legacysdk.Error
	modern := errors.As(err, &classified) && (classified.Reason == sdk.Unauthorized || classified.Reason == sdk.CredentialsExpired || classified.HTTPStatus == 401)
	legacy := errors.As(err, &previous) && (previous.Reason == legacysdk.Unauthorized || previous.Reason == legacysdk.CredentialsExpired || previous.HTTPStatus == 401)
	if !modern && !legacy {
		return
	}
	m.discard(uid, version)
}

func (m *ReadSessions) discard(uid int64, version uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, entry := range m.sessions {
		if key.userID == uid && entry.session.Version == version {
			entry.close()
			delete(m.sessions, key)
		}
	}
}

// Close 在 MCP 宿主生命周期结束时释放空闲传输。
func (m *ReadSessions) Close() error {
	// 先唤醒占有 mu的本地锁等待与在途读取，不能先取得 mu再取消生命周期。
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
	e.session.closeIdleConnections()
	e.transport.CloseIdleConnections()
}

// 原始读取租约（例如媒体 tool）在上游 401 时同样作废当前代次，不重试失败请求，
// 也不改变 SDK 的错误分类。
type readSessionTransport struct {
	inner     http.RoundTripper
	owner     *ReadSessions
	userID    int64
	version   atomic.Uint64
	revision  atomic.Int64
	rejected  atomic.Bool
	createdAt time.Time // 在发布 revision 前写入，此后只读。
	ctx       context.Context
}

func (t *readSessionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// 已失效的代次在 SDK 准备认证重放前停下，避免取消回调与新请求起跑竞争。
	if err := t.ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	// 发布前的 OAuth 请求尚无 revision；发布后的每次内容请求均复核本地凭据。
	if revision := t.revision.Load(); revision > 0 {
		a, err := t.owner.store.GetPixiv(ctx, t.userID)
		if err == nil && (a.CredentialRevision != revision || !a.CreatedAt.Equal(t.createdAt)) {
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
		if t.version.Load() != 0 {
			t.rejected.Store(true)
		}
		// 返回本次 401 的完整正文；失效只主动取消同一代次的其他传输。
		stop()
		t.owner.discard(t.userID, t.version.Load())
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

func (s ReadSession) closeIdleConnections() {
	if s.SDKClient != nil {
		s.SDKClient.CloseIdleConnections()
	} else {
		s.Client.CloseIdleConnections()
	}
}

// readCredentials 让已遭上游拒绝的代次明确返回认证失败，阻止 SDK 在同一代次刷新重放。
type readCredentials struct {
	savedCredentials
	transport *readSessionTransport
}

func (s readCredentials) Load(ctx context.Context) (pixiv.Credentials, error) {
	if s.transport.rejected.Load() {
		return pixiv.Credentials{}, &sdk.Error{Operation: "pixiv.ReadSession", Reason: sdk.CredentialsExpired, HTTPStatus: http.StatusUnauthorized, Message: "read session was rejected by upstream"}
	}
	return s.savedCredentials.Load(ctx)
}
