package accounts

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account/loginrelay"
)

// LoginAttempt binds validation and completion to the same opaque SDK login session.
// Complete must persist the account before reporting success; credentials stay with its owner.
type LoginAttempt struct {
	AuthorizationURL string
	AcceptsCallback  func(string) bool
	Complete         func(context.Context, string) (LoginResult, error)
}

// LoginResult preserves partial persistence when selecting the saved account fails.
type LoginResult struct {
	Account          Account
	AccountSaved     bool
	SelectionUpdated bool
}

// LoginStatus contains only the relay entry URL and non-secret progress/result metadata.
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

// LoginManager owns one current/recent login. Start/Close serialize replacement;
// status and HTTP routing remain available while a canceled exchange is being joined.
type LoginManager struct {
	ctx        context.Context
	cancel     context.CancelFunc
	baseURL    string
	begin      func() (LoginAttempt, error)
	operations sync.Mutex
	mu         sync.Mutex
	current    *activeLogin
}

func NewLoginManager(ctx context.Context, baseURL string, begin func() (LoginAttempt, error)) (*LoginManager, error) {
	baseURL, err := loginrelay.CanonicalPublicURL(baseURL)
	if err != nil {
		return nil, err
	}
	if begin == nil {
		return nil, errors.New("login service is not configured")
	}
	ctx, cancel := context.WithCancel(ctx)
	return &LoginManager{ctx: ctx, cancel: cancel, baseURL: baseURL, begin: begin}, nil
}

// Start reuses pending work unless restart was explicitly requested. Its lifetime
// belongs to the server context, not the MCP request that requested the login URL.
func (m *LoginManager) Start(restart bool) (LoginStatus, error) {
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

func (m *LoginManager) run(ctx context.Context, current *activeLogin, attempt LoginAttempt) {
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

func (m *LoginManager) Status(loginID string) LoginStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil || m.current.status.LoginID != loginID {
		return LoginStatus{LoginID: loginID, Status: "not_found"}
	}
	result := m.current.status
	if result.Account != nil {
		account := *result.Account
		result.Account = &account
	}
	return result
}

// ServeHTTP expects the host mux to strip the public relay path prefix.
func (m *LoginManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	current := m.current
	m.mu.Unlock()
	if current == nil {
		http.NotFound(w, r)
		return
	}
	current.relay.Handler.ServeHTTP(w, r)
}

// Close cancels and joins pending exchange/persistence without a fixed cleanup timeout.
func (m *LoginManager) Close() {
	m.cancel()
	m.operations.Lock()
	defer m.operations.Unlock()
	m.mu.Lock()
	current := m.current
	m.mu.Unlock()
	if current != nil {
		current.cancel()
		<-current.done
	}
}
