// Pixiv browser OAuth login: loopback callback server, remote relay handoff,
// and classification of the returned sign-in input.

package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/loginrelay"
	pixivaccount "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

const (
	defaultLoginAddr = "127.0.0.1:0"
)

// loginServerResult 是一次登录输入分类的最终结果：要么携带可直接用于 OAuth
// Complete 的 code，要么携带稳定、不泄露细节的错误类别。Code/Err 绝不包含
// refresh token。
type loginServerResult struct {
	Code string
	Err  error
}

// loginInputResult 在 loginServerResult 之上标记输入是否命中 Pixiv authorization
// relay 页。Relayed 为 true 时输入已交给浏览器/opener，不应再作为 code 提交。
type loginInputResult struct {
	loginServerResult
	Relayed  bool
	RelayURL string
}

// callbackURLAccepter 判定一条已提交 callback 是否属于当前登录会话。
type callbackURLAccepter = func(string) bool

type accountLoginOptions struct {
	commands.ProxyOptions
	jsonOut          bool
	noOpen           bool
	useAfterLogin    bool
	addr             string
	timeout          time.Duration
	relayPublicURL   string
	relayListenAddr  string
	relayTLSCertFile string
	relayTLSKeyFile  string
}

func (a controller) newAccountLoginCommand() *cobra.Command {
	opts := accountLoginOptions{addr: defaultLoginAddr}
	cmd := &cobra.Command{
		Use:     "login",
		Short:   "Login with the Pixiv browser OAuth flow",
		Example: "pixiv auth login --use",
		Args:    a.requireExactArgs(0, "pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--relay-public-url URL] [--relay-listen-addr ADDR]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountLogin(cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	flags.BoolVar(&opts.noOpen, "no-open", false, "do not open the browser")
	flags.StringVar(&opts.addr, "addr", defaultLoginAddr, "local loopback callback address; use 127.0.0.1:0 for an available port")
	flags.BoolVar(&opts.useAfterLogin, "use", false, "set as default account after login")
	flags.DurationVar(&opts.timeout, "timeout", 0, "maximum time to wait for login flow; 0 adds no deadline")
	flags.StringVar(&opts.relayPublicURL, "relay-public-url", "", "public URL for this remote login relay")
	flags.StringVar(&opts.relayListenAddr, "relay-listen-addr", "", "listen address for this remote login relay")
	flags.StringVar(&opts.relayTLSCertFile, "relay-tls-cert-file", "", "PEM certificate file for this remote login relay")
	flags.StringVar(&opts.relayTLSKeyFile, "relay-tls-key-file", "", "PEM private key file for this remote login relay")
	a.bindProxyFlags(cmd, &opts.ProxyOptions)
	a.bindNoInput(cmd)
	commands.Bind(cmd, commands.AuthLogin())
	return cmd
}

func (a controller) accountLogin(cmd *cobra.Command, opts accountLoginOptions) error {
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	services := a.services()
	if err := services.require(); err != nil {
		return err
	}
	if services.LoadRuntime == nil {
		return errors.New("pixiv auth runtime loader is not configured")
	}
	cfg, err := services.LoadRuntime()
	if err != nil {
		return err
	}
	if !cmd.Flags().Changed("no-open") {
		opts.noOpen = !cfg.LoginOpenBrowser
	}
	if !cmd.Flags().Changed("use") {
		opts.useAfterLogin = cfg.LoginUseAfterLogin
	}
	relayOptions, useRelay, err := configuredRelayServerOptions(cmd.Flags(), opts, cfg)
	if err != nil {
		return err
	}
	if !useRelay {
		if err := validateLoginAddr(opts.addr); err != nil {
			return err
		}
	}

	loginOptions, err := pixivOptionsForProxy(proxyOverride)
	if err != nil {
		return err
	}
	loginFlow, err := services.Login.Start(pixivaccount.LoginRequest{Options: loginOptions})
	if err != nil {
		return err
	}

	ctx := context.Background()
	var cancel context.CancelFunc
	if opts.timeout > 0 {
		// 仅在用户或配置显式要求时设置等待窗口，避免无依据打断正常授权流程。
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}

	loginURL := loginFlow.AuthorizationURL
	var callbackOrCode string
	var notifyFinal func(bool)
	var cleanupLoginServer func()
	if useRelay {
		callbackOrCode, notifyFinal, cleanupLoginServer, err = a.waitForHandoffRelayLoginCode(ctx, relayOptions, loginFlow.AcceptsCallbackURL, loginURL)
	} else {
		callbackOrCode, notifyFinal, cleanupLoginServer, err = a.waitForLoginCode(ctx, opts.addr, loginFlow.AcceptsCallbackURL, loginURL, opts.noOpen)
	}
	if err != nil {
		return err
	}
	defer cleanupLoginServer()

	result, err := services.Login.Complete(ctx, loginFlow, pixivaccount.LoginCompleteRequest{CallbackOrCode: callbackOrCode, UseAfterLogin: opts.useAfterLogin})
	if err != nil {
		// OAuth 真正失败后才向浏览器回最终失败页；页面不回显敏感原因。
		notifyFinal(false)
		return err
	}
	notifyFinal(true)

	out := accountFromPixiv(result)
	if opts.jsonOut {
		return a.printJSON(out)
	}
	// 文本模式给出可直接识别账号的安全摘要，不显示 token 或冗余的成功文案。
	fmt.Fprintf(a.out, "✓ uid:%d", out.UserID)
	if out.Username != "" {
		fmt.Fprintf(a.out, " username:%s", out.Username)
	}
	fmt.Fprintln(a.out)
	return nil
}

func (a controller) waitForLoginCode(ctx context.Context, addr string, acceptsCallback callbackURLAccepter, loginURL string, noOpen bool) (code string, notifyFinal func(bool), cleanup func(), err error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", func(bool) {}, func() {}, err
	}
	actualAddr := ln.Addr().String()
	resultCh := make(chan loginServerResult, 1)
	// finalCh 在 OAuth Complete 结束后通知仍挂起的浏览器 callback 响应。
	finalCh := make(chan bool, 1)
	var submitOnce sync.Once
	var finalOnce sync.Once
	var finalPageWaiters sync.WaitGroup
	submit := func(result loginServerResult) {
		submitOnce.Do(func() { resultCh <- result })
	}
	// notifyFinal 先通知最终页，再等待浏览器 handler 写完响应。
	// 服务器生命周期由调用方 cleanup 负责，避免过早 Shutdown 打断最终页。
	notifyFinal = func(ok bool) {
		finalOnce.Do(func() {
			select {
			case finalCh <- ok:
			default:
			}
			finalPageWaiters.Wait()
		})
	}
	// waitFinalPage 假定调用方已在 submit 之前 finalPageWaiters.Add(1)，
	// 避免 submit 放行主流程后与 Wait 竞态。
	waitFinalPage := func(w http.ResponseWriter, r *http.Request) {
		defer finalPageWaiters.Done()
		select {
		case ok := <-finalCh:
			writeLoginFinalPage(w, ok)
		case <-r.Context().Done():
			writeLoginFinalPage(w, false)
		case <-ctx.Done():
			writeLoginFinalPage(w, false)
		}
	}
	var errOutMu sync.Mutex
	writeErr := func(format string, args ...any) {
		errOutMu.Lock()
		defer errOutMu.Unlock()
		fmt.Fprintf(a.errOut, format, args...)
	}
	reportInvalidSubmission := func(err error) {
		if err != nil {
			writeErr("invalid login submission: %v\n", err)
		}
	}
	loginChallenge := pixivLoginChallenge(loginURL)
	openTerminalRelay := func(rawURL string) error {
		writeErr("Detected Pixiv authorization relay page; opening Pixiv relay URL once.\n")
		return a.openBrowser(rawURL)
	}
	if !noOpen {
		if err := a.ensureURLHandler(ctx); err != nil {
			writeErr("warning: persistent pixiv:// callback handler is unavailable: %v\n", err)
		}
		cleanup, err := a.installURLHandler(ctx, "http://"+actualAddr+"/callback")
		if err != nil {
			writeErr("warning: pixiv:// callback handler is unavailable: %v\n", err)
		} else if cleanup != nil {
			defer cleanup()
			writeErr("Registered pixiv:// callback handler for this login attempt.\n")
			writeErr("After confirming the Pixiv account, keep this terminal open while the browser shows the final result.\n")
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		writeLoginForm(w, loginURL)
	})
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.RawQuery == "" {
			// macOS helper 将 callback 放入 fragment，避免授权码进入 loopback GET 请求和浏览器历史。
			// 浏览器脚本会先清空 fragment，再通过本地 POST 提交给 /manual 并等待最终页。
			writeLoginCallbackRelayPage(w)
			return
		}
		result := loginCodeFromInput(callbackURLFromRequest(r), acceptsCallback)
		reportInvalidSubmission(result.Err)
		if result.Err != nil {
			// 输入校验失败可立即回失败页；不泄露敏感细节。
			writeLoginFinalPage(w, false)
			return
		}
		// 先登记最终页 waiter，再 submit 放行主流程，保证 notifyFinal.Wait 可见该 waiter。
		finalPageWaiters.Add(1)
		submit(result)
		// 等 OAuth 真正完成后再返回最终成功/失败页。
		waitFinalPage(w, r)
	})
	mux.HandleFunc("/manual", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeLoginForm(w, loginURL)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			reportInvalidSubmission(err)
			writeLoginFinalPage(w, false)
			return
		}
		input := r.Form.Get("login_result")
		result := classifyLoginInput(input, acceptsCallback, loginChallenge)
		if result.Relayed && result.Err == nil {
			// fallback 页面可能经 SSH 转发在另一台机器的浏览器中打开。relay 必须由
			// 当前请求的浏览器继续，不能在运行 CLI 的无 GUI 主机上调用 xdg-open。
			writeErr("Detected Pixiv authorization relay page; continuing it in the current browser.\n")
			w.Header().Set("Location", result.RelayURL)
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		reportInvalidSubmission(result.Err)
		if result.Err != nil {
			writeLoginFinalPage(w, false)
			return
		}
		// 与 /callback 相同：Add 必须先于 submit，避免与 notifyFinal 的 Wait 竞态。
		finalPageWaiters.Add(1)
		submit(result.loginServerResult)
		waitFinalPage(w, r)
	})

	server := &http.Server{Handler: mux}
	serveErr := make(chan error, 1)
	go func() {
		err := server.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErr <- err
	}()
	var cleanupOnce sync.Once
	cleanup = func() {
		cleanupOnce.Do(func() { _ = server.Shutdown(context.Background()) })
	}

	writeErr("Open this Pixiv login URL:\n%s\n", loginURL)
	if noOpen {
		writeErr("Browser opening is disabled; use the manual fallback page or terminal prompt.\n")
		if tunnelCommand, tunnelErr := loginSSHTunnelCommand(actualAddr); tunnelErr != nil {
			writeErr("warning: could not generate the SSH tunnel hint for this login listener.\n")
		} else {
			writeErr("When this CLI runs on an SSH host, forward its loopback listener from the browser machine:\n  %s\n", tunnelCommand)
		}
		writeErr("An SSH tunnel alone cannot receive Pixiv's final app link; remote browser login requires a desktop handoff.\n")
	} else {
		writeErr("Complete sign-in in your browser; the local page will receive the result.\n")
	}
	writeErr("Manual fallback page: http://%s/\n", actualAddr)

	enableTerminalFallback := a.canPrompt()
	if !noOpen {
		if err := a.openBrowser(loginURL); err != nil {
			writeErr("warning: could not open browser: %v\n", err)
		}
	}
	if enableTerminalFallback {
		go func() {
			for {
				input, err := a.promptInput("Paste the returned Pixiv sign-in address, relay address, or value", "")
				if err != nil {
					return
				}
				result := loginInputFromText(input, acceptsCallback, loginChallenge, openTerminalRelay)
				reportInvalidSubmission(result.Err)
				if result.Relayed {
					continue
				}
				if result.Err == nil {
					submit(result.loginServerResult)
					return
				}
			}
		}()
	}

	select {
	case result := <-resultCh:
		if result.Err != nil {
			cleanup()
			return "", notifyFinal, cleanup, result.Err
		}
		// 成功拿到 code 后保持服务器存活，直到调用方 notifyFinal + cleanup。
		return result.Code, notifyFinal, cleanup, nil
	case err := <-serveErr:
		cleanup()
		if err != nil {
			return "", notifyFinal, cleanup, err
		}
		return "", notifyFinal, cleanup, errors.New("login server stopped before sign-in completed")
	case <-ctx.Done():
		cleanup()
		return "", notifyFinal, cleanup, ctx.Err()
	}
}

// loginSSHTunnelCommand 只使用已绑定的 loopback listener 地址生成提示，不接受外部输入。
func loginSSHTunnelCommand(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("parse login listener address: %w", err)
	}
	if host == "" || port == "" {
		return "", errors.New("login listener address is incomplete")
	}
	return fmt.Sprintf("ssh -N -L %s:%s:%s USER@SERVER", port, host, port), nil
}

// writeLoginForm 渲染包含 Pixiv 登录链接和手动回填表单的页面。
func writeLoginForm(w http.ResponseWriter, loginURL string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := loginrelay.WriteManual(w, loginURL); err != nil {
		http.Error(w, "could not render login page", http.StatusInternalServerError)
	}
}

// writeLoginCallbackRelayPage 将 helper 传来的 fragment 安全地转为本地 POST。
// fragment 不会随 GET 发送给 loopback，脚本会在提交前将其从浏览器地址和历史记录中清除。
func writeLoginCallbackRelayPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := loginrelay.WriteCallbackRelay(w); err != nil {
		http.Error(w, "could not render login page", http.StatusInternalServerError)
	}
}

// writeLoginFinalPage 在 OAuth 真正完成后返回最终页。
// 成功/失败标题与正文均居中；失败页使用固定文案，不回显敏感原因。
func writeLoginFinalPage(w http.ResponseWriter, ok bool) {
	loginrelay.WriteFinalPage(w, ok)
}

// classifyLoginInput 只解析并校验用户提交；HTTP fallback 可以让原浏览器继续 relay，
// TTY fallback 则可选择调用本地 browser opener。两者不能混淆，否则 SSH 远端会错误地
// 尝试在无 GUI 服务器上打开 relay。
func classifyLoginInput(input string, acceptsCallback callbackURLAccepter, expectedChallenge string) loginInputResult {
	if returnTo, ok, err := pixivPostRedirectRelayURL(input, expectedChallenge); ok {
		if err != nil {
			return loginInputResult{loginServerResult: loginServerResult{Err: err}, Relayed: true}
		}
		return loginInputResult{Relayed: true, RelayURL: returnTo}
	}
	return loginInputResult{loginServerResult: loginCodeFromInput(input, acceptsCallback)}
}

// loginInputFromText 保留终端回填的既有行为：用户显式粘贴已校验 relay URL 时，
// 仅调用当前机器的 browser opener 一次。HTTP fallback 走 classifyLoginInput，
// 由提交表单的浏览器自行跳转。
func loginInputFromText(input string, acceptsCallback callbackURLAccepter, expectedChallenge string, openRelay func(string) error) loginInputResult {
	result := classifyLoginInput(input, acceptsCallback, expectedChallenge)
	if !result.Relayed || result.Err != nil {
		return result
	}
	if result.RelayURL == "" {
		return loginInputResult{loginServerResult: loginServerResult{Err: errors.New("pixiv authorization relay URL is empty")}, Relayed: true}
	}
	if openRelay == nil {
		return loginInputResult{loginServerResult: loginServerResult{Err: errors.New("browser opener is not configured")}, Relayed: true}
	}
	if err := openRelay(result.RelayURL); err != nil {
		return loginInputResult{loginServerResult: loginServerResult{Err: fmt.Errorf("could not open pixiv authorization relay URL: %w", err)}, Relayed: true}
	}
	return loginInputResult{Relayed: true, RelayURL: result.RelayURL}
}

// loginCodeFromInput 对一条登录输入做最终分类：合法 browser/app callback 携带
// code；非法或与当前会话不匹配的输入只返回稳定错误类别。
func loginCodeFromInput(input string, acceptsCallback callbackURLAccepter) loginServerResult {
	input = strings.TrimSpace(input)
	if input == "" {
		return loginServerResult{Err: errors.New("sign-in result cannot be empty")}
	}
	if strings.Contains(input, "://") || strings.HasPrefix(input, "/") {
		parsed, err := url.Parse(input)
		if err != nil {
			return loginServerResult{Err: errors.New("invalid sign-in address")}
		}
		if parsed.RawQuery != "" {
			if acceptsCallback == nil || !acceptsCallback(input) {
				return loginServerResult{Err: errors.New("sign-in address does not match this login session")}
			}
			return loginServerResult{Code: input}
		}
		return loginServerResult{Err: errors.New("sign-in address did not include required details")}
	}
	return loginServerResult{Code: input}
}

func callbackURLFromRequest(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	return scheme + "://" + host + r.URL.RequestURI()
}

func pixivLoginChallenge(loginURL string) string {
	parsed, err := url.Parse(loginURL)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get("code_challenge"))
}

func validateLoginAddr(addr string) error {
	if strings.TrimSpace(addr) == "" {
		return errors.New("--addr cannot be empty")
	}
	if addr == defaultLoginAddr {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --addr %q: %w", addr, err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("--addr must bind to a loopback address, got %q", addr)
	}
	return nil
}

// pixivPostRedirectReturnTo 只接受指向官方 OAuth start 页的 post-redirect relay。
// 返回的 return_to 仍是 Pixiv 域内 URL，不包含任何授权码或 token。
func pixivPostRedirectReturnTo(rawURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(parsed.Scheme, "https") || !strings.EqualFold(parsed.Host, "accounts.pixiv.net") || parsed.Path != "/post-redirect" {
		return "", false
	}
	returnTo := strings.TrimSpace(parsed.Query().Get("return_to"))
	if returnTo == "" {
		return "", false
	}
	target, err := url.Parse(returnTo)
	if err != nil {
		return "", false
	}
	if !pixiv.IsOfficialOAuthStartURL(target.String()) {
		return "", false
	}
	return returnTo, true
}

func pixivPostRedirectRelayURL(rawURL, expectedChallenge string) (string, bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", false, nil
	}
	if !strings.EqualFold(parsed.Scheme, "https") || !strings.EqualFold(parsed.Host, "accounts.pixiv.net") || parsed.Path != "/post-redirect" {
		return "", false, nil
	}
	returnTo, ok := pixivPostRedirectReturnTo(rawURL)
	if !ok {
		return "", true, errors.New("invalid Pixiv authorization relay URL")
	}
	if !pixivAuthStartMatchesChallenge(returnTo, expectedChallenge) {
		return "", true, errors.New("Pixiv authorization relay URL does not match this login attempt")
	}
	return strings.TrimSpace(rawURL), true, nil
}

// pixivAuthStartMatchesChallenge 校验 relay URL 的 code_challenge 是否仍对应
// 当前登录尝试。
func pixivAuthStartMatchesChallenge(rawURL, expectedChallenge string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return parsed.Query().Get("code_challenge") == expectedChallenge
}
