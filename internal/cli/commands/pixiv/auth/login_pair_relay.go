package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth/loginhelper"
)

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

// WaitForHandoffRelayLoginCode 在独立 server 上运行一次性 remote Pixiv login
// relay 会话，并把会话页 URL 写入 errOut。errOut 只接收会话 URL，绝不含 OAuth
// 凭证；loginURL 不写入任何输出。该函数是 accountLogin 的 relay 分支与聚焦
// handoff 测试共用的入口。
func WaitForHandoffRelayLoginCode(ctx context.Context, errOut io.Writer, opts RelayServerOptions, acceptsCallback CallbackURLAccepter, loginURL string) (string, func(bool), func(), error) {
	return controller{errOut: errOut}.waitForHandoffRelayLoginCode(ctx, opts, acceptsCallback, loginURL)
}

// waitForHandoffRelayLoginCode 在 server 上运行一次性会话。会话 proof 只存在
// session page 的操作入口与 desktop 的短暂私有状态中，终端不输出 Pixiv OAuth URL。
func (a controller) waitForHandoffRelayLoginCode(ctx context.Context, opts RelayServerOptions, acceptsCallback CallbackURLAccepter, loginURL string) (string, func(bool), func(), error) {
	publicURL, err := canonicalHandoffRelayPublicURL(opts.PublicURL)
	if err != nil {
		return "", func(bool) {}, func() {}, err
	}
	listen := opts.Listen
	if listen == nil {
		listen = net.Listen
	}
	listener, err := listen("tcp", opts.ListenAddr)
	if err != nil {
		return "", func(bool) {}, func() {}, err
	}
	sessionID, err := newRelayResultID()
	if err != nil {
		_ = listener.Close()
		return "", func(bool) {}, func() {}, err
	}
	proof, err := newRelayResultID()
	if err != nil {
		_ = listener.Close()
		return "", func(bool) {}, func() {}, err
	}
	resultID, err := newRelayResultID()
	if err != nil {
		_ = listener.Close()
		return "", func(bool) {}, func() {}, err
	}
	sessionURL, err := handoffRelayURL(publicURL, "session", sessionID)
	if err != nil {
		_ = listener.Close()
		return "", func(bool) {}, func() {}, err
	}
	resultURL, err := handoffRelayURL(publicURL, "result", resultID)
	if err != nil {
		_ = listener.Close()
		return "", func(bool) {}, func() {}, err
	}
	startURL := handoffRelayDeepLink(publicURL, sessionID, proof)

	resultCh := make(chan LoginServerResult, 1)
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
	go func() {
		defer func() {
			if opts.ContextWaiterExited != nil {
				opts.ContextWaiterExited()
			}
		}()
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
		if !loginhelper.IsAllowedPixivCallbackURL(raw) {
			return errors.New("invalid Pixiv login result")
		}
		result := LoginCodeFromInput(raw, acceptsCallback)
		if result.Err != nil {
			return errors.New("login result does not match this session")
		}
		sessionMu.Lock()
		defer sessionMu.Unlock()
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
		resultCh <- result
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
		alreadySubmitted := submitted
		started = true
		sessionMu.Unlock()
		if alreadySubmitted {
			http.Error(w, "login result has already been received", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(loginhelper.RemoteLoginStartResponse{AuthorizationURL: loginURL})
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
			case "login result has already been received", "remote login session is not ready":
				http.Error(w, err.Error(), http.StatusConflict)
			default:
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
			return
		}
		w.Header().Set(loginhelper.RelayResultURLHeader, resultURL)
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
			WriteLoginFinalPage(w, ok)
		case <-r.Context().Done():
			WriteLoginFinalPage(w, false)
		case <-ctx.Done():
			WriteLoginFinalPage(w, false)
		}
	})

	server := &http.Server{Handler: mux}
	serveErr := make(chan error, 1)
	go func() {
		var serveErrValue error
		if opts.TLSCertFile != "" {
			serveErrValue = server.ServeTLS(listener, opts.TLSCertFile, opts.TLSKeyFile)
		} else {
			serveErrValue = server.Serve(listener)
		}
		if errors.Is(serveErrValue, http.ErrServerClosed) {
			serveErrValue = nil
		}
		serveErr <- serveErrValue
	}()
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			stopContextWaiter()
			_ = server.Shutdown(context.Background())
		})
	}
	fmt.Fprintf(a.errOut, "Remote Pixiv login relay is listening on %s.\n", listener.Addr().String())
	fmt.Fprintf(a.errOut, "Open remote Pixiv login session:\n%s\n", sessionURL)

	select {
	case result := <-resultCh:
		// callback 已由本次会话接收，后续由 notifyFinal 负责结果页；不再需要一个
		// 仅等待父 context 的 goroutine，否则成功登录会一直保留至进程结束。
		stopContextWaiter()
		return result.Code, notifyFinal, cleanup, result.Err
	case err := <-serveErr:
		cleanup()
		if err != nil {
			// ServeTLS 的底层错误会包含 PEM 的绝对路径，不能越过 CLI 错误边界。
			return "", func(bool) {}, cleanup, errors.New("remote login relay server failed; verify its listener and TLS configuration")
		}
		return "", func(bool) {}, cleanup, errors.New("remote login relay stopped before sign-in completed")
	case <-ctx.Done():
		cleanup()
		return "", func(bool) {}, cleanup, ctx.Err()
	}
}

func newRelayResultID() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:]), nil
}

func handoffRelayURL(publicURL, segment, id string) (string, error) {
	canonical, err := canonicalHandoffRelayPublicURL(publicURL)
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(canonical)
	parsed.Path = path.Join("/", parsed.Path, segment, id)
	return parsed.String(), nil
}

// canonicalHandoffRelayPublicURL 在启动 session 前一次性规范 relay base；随后
// session、manual/result endpoint 与 deep link 都只使用这一值，不能各自重写。
func canonicalHandoffRelayPublicURL(raw string) (string, error) {
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
