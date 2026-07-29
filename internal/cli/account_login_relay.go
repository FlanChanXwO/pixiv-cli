package cli

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/loginhelper"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
)

type loginRelayServerOptions struct {
	PublicURL   string
	ListenAddr  string
	Secret      string
	TLSCertFile string
	TLSKeyFile  string
}

type relayLoginCallbackRequest struct {
	CallbackURL string `json:"callback_url"`
}

// relayLoginFinalResponse 是 callback 长连接结束时返回给本机 handler 的最小结果。
// 浏览器只会看到独立的固定成功/失败页，永远不接触 token、code 或服务端诊断。
type relayLoginFinalResponse struct {
	Success bool `json:"success"`
}

// configuredRelayServerOptions 按单次 flag > 私有 config 的优先级合成 server
// relay；没有任一 server 值时保持普通本地登录。client target 本身不触发 server。
func configuredRelayServerOptions(cmd flagChangeReader, opts accountLoginOptions, cfg config.RuntimeConfig) (loginRelayServerOptions, bool, error) {
	result := loginRelayServerOptions{
		PublicURL:   cfg.LoginRelayPublicURL,
		ListenAddr:  cfg.LoginRelayListenAddr,
		Secret:      cfg.LoginRelaySecret,
		TLSCertFile: cfg.LoginRelayTLSCertFile,
		TLSKeyFile:  cfg.LoginRelayTLSKeyFile,
	}
	if cmd.Changed("relay-public-url") {
		result.PublicURL = opts.relayPublicURL
	}
	if cmd.Changed("relay-listen-addr") {
		result.ListenAddr = opts.relayListenAddr
	}
	if cmd.Changed("relay-tls-cert-file") {
		result.TLSCertFile = opts.relayTLSCertFile
	}
	if cmd.Changed("relay-tls-key-file") {
		result.TLSKeyFile = opts.relayTLSKeyFile
	}
	configured := result.PublicURL != "" || result.ListenAddr != "" || result.Secret != "" || result.TLSCertFile != "" || result.TLSKeyFile != ""
	if !configured {
		return loginRelayServerOptions{}, false, nil
	}
	if result.PublicURL == "" || result.ListenAddr == "" || result.Secret == "" {
		return loginRelayServerOptions{}, false, errors.New("remote login relay requires login_relay_public_url, login_relay_listen_addr, and login_relay_secret")
	}
	if (result.TLSCertFile == "") != (result.TLSKeyFile == "") {
		return loginRelayServerOptions{}, false, errors.New("remote login relay TLS requires both certificate and key files")
	}
	if err := validateRelayServerOptions(result); err != nil {
		return loginRelayServerOptions{}, false, err
	}
	return result, true, nil
}

// flagChangeReader 让 relay 配置选择不依赖 cobra，便于不启动实际 listener 的单测。
type flagChangeReader interface{ Changed(string) bool }

func validateRelayServerOptions(opts loginRelayServerOptions) error {
	publicURL, err := url.Parse(strings.TrimSpace(opts.PublicURL))
	if err != nil || publicURL == nil || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return errors.New("invalid remote login relay public URL")
	}
	host, _, err := net.SplitHostPort(opts.ListenAddr)
	if err != nil || strings.TrimSpace(host) == "" {
		return errors.New("remote login relay listen address must include host and port")
	}
	if publicURL.Scheme == "https" && opts.TLSCertFile == "" && !isLoopbackHost(host) {
		return errors.New("HTTPS relay without TLS PEM must listen on loopback for a same-host reverse proxy")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (a app) waitForRelayLoginCode(ctx context.Context, opts loginRelayServerOptions, acceptsCallback callbackURLAccepter, loginURL string) (string, func(bool), func(), error) {
	listener, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return "", func(bool) {}, func() {}, err
	}
	resultID, err := newRelayResultID()
	if err != nil {
		_ = listener.Close()
		return "", func(bool) {}, func() {}, err
	}
	resultURL, err := relayResultURL(opts.PublicURL, resultID)
	if err != nil {
		_ = listener.Close()
		return "", func(bool) {}, func() {}, err
	}
	resultPath := path.Join("/result", resultID)
	resultCh := make(chan loginServerResult, 1)
	var submitted bool
	var submittedMu sync.Mutex
	// callback 请求先把 result-page waiter 登记好，再放行 OAuth Complete。这样
	// Complete 结束时既不会抢在浏览器 GET 前关闭 listener，也不会错过最终页。
	var resultPageWaiters sync.WaitGroup
	var resultPageOnce sync.Once
	var finalOnce sync.Once
	var finalStatusMu sync.RWMutex
	finalStatus := false
	finalReady := make(chan struct{})
	finalPageWritten := make(chan struct{})
	finishResultPage := func() {
		resultPageOnce.Do(func() { resultPageWaiters.Done() })
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

	authorized := func(r *http.Request) bool {
		const prefix = "Bearer "
		candidate := strings.TrimPrefix(r.Header.Get("Authorization"), prefix)
		if candidate == r.Header.Get("Authorization") {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(candidate), []byte(opts.Secret)) == 1
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		submittedMu.Lock()
		alreadySubmitted := submitted
		submittedMu.Unlock()
		if alreadySubmitted {
			http.Error(w, "login callback has already been received", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var request relayLoginCallbackRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !loginhelper.IsAllowedPixivCallbackURL(request.CallbackURL) {
			http.Error(w, "invalid Pixiv callback", http.StatusBadRequest)
			return
		}
		result := loginCodeFromInput(request.CallbackURL, acceptsCallback)
		if result.err != nil {
			http.Error(w, "callback does not match this login session", http.StatusBadRequest)
			return
		}
		submittedMu.Lock()
		if submitted {
			submittedMu.Unlock()
			http.Error(w, "login callback has already been received", http.StatusConflict)
			return
		}
		submitted = true
		submittedMu.Unlock()
		// result URL 在已认证且 state 匹配后才产生。它是随机的一次性页面
		// capability，只显示固定最终结果，不携带授权码或 token。
		resultPageWaiters.Add(1)
		resultCh <- result
		w.Header().Set(loginhelper.RelayResultURLHeader, resultURL)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		// 本机 handler 已拿到 header 后会打开 resultURL；保持这条认证请求直到
		// OAuth exchange 真正结束，避免它把“已接收 callback”误当作登录成功。
		select {
		case <-finalPageWritten:
			finalStatusMu.RLock()
			ok := finalStatus
			finalStatusMu.RUnlock()
			_ = json.NewEncoder(w).Encode(relayLoginFinalResponse{Success: ok})
		case <-r.Context().Done():
			// handler 无法启动浏览器或主动取消时，释放 waiter，不能让 server
			// 因一个不会到达的结果页永久滞留。
			finishResultPage()
		case <-ctx.Done():
			finishResultPage()
		}
	})
	mux.HandleFunc(resultPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
			writeLoginFinalPage(w, ok)
		case <-r.Context().Done():
			writeLoginFinalPage(w, false)
		case <-ctx.Done():
			writeLoginFinalPage(w, false)
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
	cleanup := func() { cleanupOnce.Do(func() { _ = server.Shutdown(context.Background()) }) }
	if strings.HasPrefix(strings.ToLower(opts.PublicURL), "http://") {
		fmt.Fprintln(a.errOut, "warning: remote Pixiv login relay uses HTTP; the callback and relay secret can be observed or modified by the network.")
	}
	fmt.Fprintf(a.errOut, "Remote Pixiv login relay is listening on %s.\n", listener.Addr().String())
	fmt.Fprintf(a.errOut, "Open this Pixiv login URL in the browser on the configured client machine:\n%s\n", loginURL)

	select {
	case result := <-resultCh:
		if result.err != nil {
			cleanup()
			return "", func(bool) {}, cleanup, result.err
		}
		return result.code, notifyFinal, cleanup, nil
	case err := <-serveErr:
		cleanup()
		if err != nil {
			return "", func(bool) {}, cleanup, err
		}
		return "", func(bool) {}, cleanup, errors.New("remote login relay stopped before receiving authorization code")
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

// relayResultURL 使用与 callback 相同的 public base URL 生成一次性最终页地址。
// 即使配置后来被代理到 path prefix，客户端也只会打开该受验证 base 下的 result 路径。
func relayResultURL(publicURL, resultID string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid remote login relay public URL")
	}
	if _, err := base64.RawURLEncoding.DecodeString(resultID); err != nil {
		return "", errors.New("invalid remote login relay result identifier")
	}
	parsed.Path = path.Join("/", parsed.Path, "result", resultID)
	return parsed.String(), nil
}
