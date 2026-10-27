package accounts

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/loginrelay"
)

// LoginAttempt 把回调校验与完成动作绑定到同一个 SDK 登录会话。Complete 必须先持久化
// 账号再返回成功；凭据始终留在账号存储里。
type LoginAttempt struct {
	AuthorizationURL string
	AcceptsCallback  func(string) bool
	Complete         func(context.Context, string) (LoginResult, error)
}

// LoginResult 保留部分成功的结果：账号可能已保存，而选择账号这一步失败。
type LoginResult struct {
	Account          Account
	AccountSaved     bool
	SelectionUpdated bool
}

// LoginStatus 只含接力入口 URL 与不含凭据的进度、结果信息。
type LoginStatus struct {
	LoginID          string   `json:"login_id"`
	AuthorizationURL string   `json:"authorization_url,omitempty"`
	Status           string   `json:"status"`
	Account          *Account `json:"account,omitempty"`
	AccountSaved     bool     `json:"account_saved"`
	SelectionUpdated bool     `json:"selection_updated"`
	ErrorCode        string   `json:"error_code,omitempty"`
}

type activeLogin struct {
	relay  *loginrelay.Session
	cancel context.CancelFunc
	done   chan struct{}
	status LoginStatus
}

// Login 维护当前或最近一次登录接力。Start 与 Close 串行化替换过程；旧的兑换正在收尾时，
// 状态查询与 HTTP 路由仍可访问新的接力。
type Login struct {
	ctx        context.Context
	cancel     context.CancelFunc
	baseURL    string
	begin      func() (LoginAttempt, error)
	operations sync.Mutex
	mu         sync.Mutex
	current    *activeLogin
}

// NewLogin 校验公开 base URL 并返回归属 ctx 的登录管理器；Close 会取消尚未完成的接力。
func NewLogin(ctx context.Context, baseURL string, begin func() (LoginAttempt, error)) (*Login, error) {
	baseURL, err := loginrelay.CanonicalPublicURL(baseURL)
	if err != nil {
		return nil, err
	}
	if begin == nil {
		return nil, errors.New("login service is not configured")
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Login{ctx: ctx, cancel: cancel, baseURL: baseURL, begin: begin}, nil
}

// Start 在未显式要求 restart 时复用进行中的登录。接力的生命周期属于 server context，
// 不会随发起请求的 MCP 调用结束。
func (m *Login) Start(restart bool) (LoginStatus, error) {
	m.operations.Lock()
	defer m.operations.Unlock()
	if err := m.ctx.Err(); err != nil {
		return LoginStatus{}, err
	}
	m.mu.Lock()
	old := m.current
	if old != nil && !restart && (old.status.Status == "waiting_for_user" || old.status.Status == "exchanging") {
		result := old.status
		m.mu.Unlock()
		return result, nil
	}
	m.mu.Unlock()
	if old != nil {
		old.cancel()
		<-old.done
	}
	m.mu.Lock()
	m.current = nil
	m.mu.Unlock()
	if err := m.ctx.Err(); err != nil {
		return LoginStatus{}, err
	}
	attempt, err := m.begin()
	if err != nil {
		return LoginStatus{}, err
	}
	if attempt.AcceptsCallback == nil || attempt.Complete == nil {
		return LoginStatus{}, errors.New("login service is not configured")
	}
	ctx, cancel := context.WithCancel(m.ctx)
	relay, err := loginrelay.New(ctx, m.baseURL, attempt.AuthorizationURL, attempt.AcceptsCallback)
	if err != nil {
		cancel()
		return LoginStatus{}, err
	}
	if err := ctx.Err(); err != nil {
		cancel()
		relay.Stop()
		return LoginStatus{}, err
	}
	current := &activeLogin{relay: relay, cancel: cancel, done: make(chan struct{}), status: LoginStatus{LoginID: rand.Text(), AuthorizationURL: relay.URL, Status: "waiting_for_user"}}
	m.mu.Lock()
	m.current = current
	result := current.status
	m.mu.Unlock()
	go m.run(ctx, current, attempt)
	return result, nil
}

func (m *Login) run(ctx context.Context, current *activeLogin, attempt LoginAttempt) {
	defer close(current.done)
	defer current.relay.Stop()
	var callback string
	select {
	case callback = <-current.relay.Callback:
	case <-ctx.Done():
		m.mu.Lock()
		current.status.Status, current.status.ErrorCode = "failed", "login_cancelled"
		m.mu.Unlock()
		return
	}
	current.relay.Stop()
	m.mu.Lock()
	current.status.Status = "exchanging"
	m.mu.Unlock()
	var result LoginResult
	err := ctx.Err()
	if err == nil {
		result, err = attempt.Complete(ctx, callback)
	}
	m.mu.Lock()
	current.status.AccountSaved = result.AccountSaved
	current.status.SelectionUpdated = result.SelectionUpdated
	if result.AccountSaved {
		account := result.Account
		current.status.Account = &account
	}
	current.status.Status = "completed"
	if err != nil {
		current.status.Status, current.status.ErrorCode = "failed", "login_failed"
		if ctx.Err() != nil {
			current.status.ErrorCode = "login_cancelled"
		}
	}
	m.mu.Unlock()
	current.relay.Complete(err == nil)
}

// Status 返回与该 loginID 匹配的最近一次登录状态；ID 不匹配时返回 not_found。
func (m *Login) Status(loginID string) LoginStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil || m.current.status.LoginID != loginID {
		return LoginStatus{LoginID: loginID, Status: "not_found"}
	}
	return m.current.status
}

// ServeHTTP 要求外层 mux 先剥掉公开的 relay 路径前缀。
func (m *Login) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	current := m.current
	m.mu.Unlock()
	if current == nil {
		http.NotFound(w, r)
		return
	}
	current.relay.Handler.ServeHTTP(w, r)
}

// Close 取消并等待进行中的兑换与持久化，不设固定清理超时。
func (m *Login) Close() {
	m.cancel()
	m.operations.Lock()
	defer m.operations.Unlock()
	m.mu.Lock()
	current := m.current
	m.mu.Unlock()
	if current != nil {
		<-current.done
	}
}
