package loginrelay

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account/loginrelay/loginpage"
)

// Session 提供单次登录的 HTTP handler，不创建 listener 或操作账号库。
// 调用方负责用同一 OAuth session 校验 callback、兑换凭据并调用 Complete。
// Handler 的路径相对 relay base；挂载在主 mux 时由调用方剥离公开路径前缀。
type Session struct {
	URL      string
	Handler  http.Handler
	Callback <-chan string
	Complete func(bool)
	// Stop 释放父 context waiter；在收到 Callback 后或放弃会话时调用。
	// HTTP 请求仍由创建时的 context 和外层 server 生命周期控制。
	Stop func()
	// Done 在父 context waiter 退出后关闭，供 owner 确认清理完成。
	Done <-chan struct{}
}

const inactiveSessionMessage = "remote login session is no longer active"

type remoteLoginStartRequest struct {
	Proof string `json:"proof"`
}

type relayLoginCallbackRequest struct {
	CallbackURL string `json:"callback_url"`
	Proof       string `json:"proof"`
}

// relayLoginFinalResponse 是 callback 长连接结束时返回给本机 handler 的最小结果。
// 浏览器只会看到独立的固定成功/失败页，永远不接触 token、code 或服务端诊断。
type relayLoginFinalResponse struct {
	Success bool `json:"success"`
}

// New 创建可挂载的 relay 会话。acceptsCallback 必须校验本次 OAuth 会话；
// Callback 只交付一次经过校验的 URL，不将请求断线视为凭据兑换取消。
func New(ctx context.Context, publicURL, loginURL string, acceptsCallback func(string) bool) (*Session, error) {
	publicURL, err := CanonicalPublicURL(publicURL)
	if err != nil {
		return nil, err
	}
	sessionID, err := newRelayResultID()
	if err != nil {
		return nil, err
	}
	proof, err := newRelayResultID()
	if err != nil {
		return nil, err
	}
	resultID, err := newRelayResultID()
	if err != nil {
		return nil, err
	}
	sessionURL, err := handoffRelayURL(publicURL, "session", sessionID)
	if err != nil {
		return nil, err
	}
	resultURL, err := handoffRelayURL(publicURL, "result", resultID)
	if err != nil {
		return nil, err
	}
	startURL := handoffRelayDeepLink(publicURL, sessionID, proof)

	resultCh := make(chan string, 1)
	var sessionMu sync.Mutex
	started := false
	submitted := false
	var resultPageWaiters sync.WaitGroup
	var resultPageOnce sync.Once
	var resultWaiterMu sync.Mutex
	resultWaiterAdded := false
	var finalOnce sync.Once
	var finalStatusMu sync.RWMutex
	finalStatus := false
	finalReady := make(chan struct{})
	finalPageWritten := make(chan struct{})
	contextWaiterStopped := make(chan struct{})
	var stopContextWaiterOnce sync.Once
	stopContextWaiter := func() {
		stopContextWaiterOnce.Do(func() { close(contextWaiterStopped) })
	}
	finishResultPage := func() {
		resultWaiterMu.Lock()
		added := resultWaiterAdded
		resultWaiterMu.Unlock()
		if added {
			resultPageOnce.Do(func() { resultPageWaiters.Done() })
		}
	}
	notifyFinal := func(ok bool) {
		finalOnce.Do(func() {
			finalStatusMu.Lock()
			finalStatus = ok
			finalStatusMu.Unlock()
			close(finalReady)
			resultPageWaiters.Wait()
			close(finalPageWritten)
		})
	}
	waiterDone := make(chan struct{})
	go func() {
		defer close(waiterDone)
		select {
		case <-ctx.Done():
			finishResultPage()
		case <-contextWaiterStopped:
		}
	}()

	proofMatches := func(candidate string) bool {
		candidate = strings.TrimSpace(candidate)
		return candidate != "" && subtle.ConstantTimeCompare([]byte(candidate), []byte(proof)) == 1
	}
	submitCallback := func(raw string) error {
		if !IsAllowedPixivCallbackURL(raw) {
			return errors.New("invalid Pixiv login result")
		}
		callback := strings.TrimSpace(raw)
		if acceptsCallback == nil || !acceptsCallback(callback) {
			return errors.New("login result does not match this session")
		}
		sessionMu.Lock()
		defer sessionMu.Unlock()
		// restart/shutdown 可能发生在 SDK 校验期间；取消的会话不能再交付凭据。
		if ctx.Err() != nil {
			return errors.New(inactiveSessionMessage)
		}
		if !started {
			return errors.New("remote login session is not ready")
		}
		if submitted {
			return errors.New("login result has already been received")
		}
		submitted = true
		resultPageWaiters.Add(1)
		resultWaiterMu.Lock()
		resultWaiterAdded = true
		resultWaiterMu.Unlock()
		resultCh <- callback
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/session/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !handoffRelayPathMatches(r.URL.Path, "session", sessionID) {
			http.NotFound(w, r)
			return
		}
		// session URL 是一次性 desktop handoff 的入口，不再渲染项目中间页。
		// 浏览器直接交给当前用户注册的 pixiv:// handler；该 handler 领取 OAuth URL
		// 并把官方 callback 回传到本次 server 会话。
		w.Header().Set("Location", startURL)
		w.WriteHeader(http.StatusSeeOther)
	})
	mux.HandleFunc("/start/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !handoffRelayPathMatches(r.URL.Path, "start", sessionID) {
			http.NotFound(w, r)
			return
		}
		var request remoteLoginStartRequest
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !proofMatches(request.Proof) {
			http.Error(w, "invalid remote login session", http.StatusUnauthorized)
			return
		}
		sessionMu.Lock()
		if ctx.Err() != nil {
			sessionMu.Unlock()
			http.Error(w, inactiveSessionMessage, http.StatusGone)
			return
		}
		alreadySubmitted := submitted
		started = true
		sessionMu.Unlock()
		if alreadySubmitted {
			http.Error(w, "login result has already been received", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RemoteLoginStartResponse{AuthorizationURL: loginURL})
	})
	mux.HandleFunc("/callback/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !handoffRelayPathMatches(r.URL.Path, "callback", sessionID) {
			http.NotFound(w, r)
			return
		}
		var request relayLoginCallbackRequest
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !proofMatches(request.Proof) {
			http.Error(w, "invalid remote login session", http.StatusUnauthorized)
			return
		}
		if err := submitCallback(request.CallbackURL); err != nil {
			switch err.Error() {
			case inactiveSessionMessage:
				http.Error(w, inactiveSessionMessage, http.StatusGone)
			case "login result has already been received", "remote login session is not ready":
				http.Error(w, err.Error(), http.StatusConflict)
			default:
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
			return
		}
		w.Header().Set(RelayResultURLHeader, resultURL)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-finalPageWritten:
			finalStatusMu.RLock()
			ok := finalStatus
			finalStatusMu.RUnlock()
			_ = json.NewEncoder(w).Encode(relayLoginFinalResponse{Success: ok})
		case <-r.Context().Done():
			finishResultPage()
		case <-ctx.Done():
			finishResultPage()
		}
	})
	mux.HandleFunc("/result/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !handoffRelayPathMatches(r.URL.Path, "result", resultID) {
			http.NotFound(w, r)
			return
		}
		claimed := false
		resultPageOnce.Do(func() { claimed = true })
		if !claimed {
			http.Error(w, "login result has already been opened", http.StatusConflict)
			return
		}
		defer resultPageWaiters.Done()
		select {
		case <-finalReady:
			finalStatusMu.RLock()
			ok := finalStatus
			finalStatusMu.RUnlock()
			WriteFinalPage(w, ok)
		case <-r.Context().Done():
			WriteFinalPage(w, false)
		case <-ctx.Done():
			WriteFinalPage(w, false)
		}
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil {
			http.Error(w, inactiveSessionMessage, http.StatusGone)
			return
		}
		mux.ServeHTTP(w, r)
	})
	return &Session{URL: sessionURL, Handler: handler, Callback: resultCh, Complete: notifyFinal, Stop: stopContextWaiter, Done: waiterDone}, nil
}

func newRelayResultID() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:]), nil
}

func handoffRelayURL(publicURL, segment, id string) (string, error) {
	canonical, err := CanonicalPublicURL(publicURL)
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(canonical)
	escapedBase := parsed.EscapedPath()
	parsed.Path = path.Join("/", parsed.Path, segment, id)
	parsed.RawPath = path.Join("/", escapedBase, url.PathEscape(segment), url.PathEscape(id))
	return parsed.String(), nil
}

// CanonicalPublicURL 在启动 session 前一次性规范 relay base；随后
// session、manual/result endpoint 与 deep link 都只使用这一值，不能各自重写。
func CanonicalPublicURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid remote login relay public URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path != "" {
		parsed.Path = path.Clean("/" + parsed.Path)
		if parsed.Path == "/" {
			parsed.Path = ""
		}
	}
	return parsed.String(), nil
}

func handoffRelayPathMatches(rawPath, segment, id string) bool {
	return rawPath == path.Join("/", segment, id)
}

func handoffRelayDeepLink(origin, sessionID, proof string) string {
	values := url.Values{"origin": {origin}, "session": {sessionID}, "access": {proof}}
	return (&url.URL{Scheme: "pixiv", Host: "account", Path: "/remote-login", RawQuery: values.Encode()}).String()
}

// WriteFinalPage 仅渲染固定成功/失败文案，不回显凭据或服务端诊断。
func WriteFinalPage(w http.ResponseWriter, ok bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
	}
	if err := loginpage.WriteResult(w, ok); err != nil {
		http.Error(w, "could not render login page", http.StatusInternalServerError)
	}
}
