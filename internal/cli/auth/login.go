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

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/loginrelay"
	"github.com/FlanChanXwO/pixiv-cli/internal/urlscheme"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

const (
	defaultLoginAddr = "127.0.0.1:0"
)

// loginServerResult 保存一次登录输入分类的最终结果：要么携带可直接交给 OAuth
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
	environment.ProxyOptions
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
		Short:   "help.login_with_the_pixiv_browser_oauth_flow",
		Example: "pixiv auth login --use",
		Args:    a.env.ExactArgs(0, "pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--relay-public-url URL] [--relay-listen-addr ADDR]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountLogin(cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "flag.print_json")
	flags.BoolVar(&opts.noOpen, "no-open", false, "flag.do_not_open_the_browser")
	flags.StringVar(&opts.addr, "addr", defaultLoginAddr, "flag.local_loopback_callback_address_use_127_0_0")
	flags.BoolVar(&opts.useAfterLogin, "use", false, "flag.set_as_default_account_after_login")
	flags.DurationVar(&opts.timeout, "timeout", 0, "flag.maximum_time_to_wait_for_login_flow_0")
	flags.StringVar(&opts.relayPublicURL, "relay-public-url", "", "flag.public_url_for_this_remote_login_relay")
	flags.StringVar(&opts.relayListenAddr, "relay-listen-addr", "", "flag.listen_address_for_this_remote_login_relay")
	flags.StringVar(&opts.relayTLSCertFile, "relay-tls-cert-file", "", "flag.pem_certificate_file_for_this_remote_login_relay")
	flags.StringVar(&opts.relayTLSKeyFile, "relay-tls-key-file", "", "flag.pem_private_key_file_for_this_remote_login")
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	a.env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a controller) accountLogin(cmd *cobra.Command, opts accountLoginOptions) error {
	proxyOverride, err := environment.ProxyOverride(cmd, opts.ProxyOptions)
	if err != nil {
		return err
	}
	login, err := a.loginService(cmd.Context())
	if err != nil {
		return err
	}
	cfg, err := a.support.LoadRuntime()
	if err != nil {
		return err
	}
	if !cmd.Flags().Changed("no-open") {
		opts.noOpen = !cfg.Login.OpenBrowser
	}
	if !cmd.Flags().Changed("use") {
		opts.useAfterLogin = cfg.Login.UseAfterLogin
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
	if loginOptions.HTTPClient != nil {
		defer loginOptions.HTTPClient.CloseIdleConnections()
	}
	loginFlow, err := login.Start(account.LoginRequest{Options: loginOptions})
	if err != nil {
		return err
	}

	ctx := cmd.Context()
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

	result, err := login.Complete(ctx, loginFlow, LoginCompleteRequest{CallbackOrCode: callbackOrCode, UseAfterLogin: opts.useAfterLogin})
	if err != nil {
		// OAuth 真正失败后才向浏览器回最终失败页；页面不回显敏感原因。
		notifyFinal(false)
		return err
	}
	notifyFinal(true)

	out := accountFromPixiv(result)
	if opts.jsonOut {
		return a.env.PrintJSON(out)
	}
	_, err = fmt.Fprintf(a.env.Out, a.env.Text("output.logged_in_as"), display.Identity(out.Username, out.UserID))
	return err
}

func (a controller) waitForLoginCode(ctx context.Context, addr string, acceptsCallback callbackURLAccepter, loginURL string, noOpen bool) (code string, notifyFinal func(bool), cleanup func(), err error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", func(bool) {}, func() {}, err
	}
	login := newLoginServer(a, ctx, listener, acceptsCallback, loginURL)
	login.notifyFinal = login.notifyFinalPage
	if !noOpen {
		login.installHandler(ctx)
	}
	login.serve(loginURL, noOpen)
	select {
	case result := <-login.resultCh:
		if result.Err != nil {
			login.cleanup()
			return "", login.notifyFinal, login.cleanup, result.Err
		}
		return result.Code, login.notifyFinal, login.cleanup, nil
	case err := <-login.serveErr:
		login.cleanup()
		if err != nil {
			return "", login.notifyFinal, login.cleanup, err
		}
		return "", login.notifyFinal, login.cleanup, i18n.Errorf("error.login_server_stopped_before_sign_in_completed")
	case <-ctx.Done():
		login.cleanup()
		return "", login.notifyFinal, login.cleanup, ctx.Err()
	}
}

// loginServer 持有一个 loopback 回调监听器和它面向浏览器的页面；每个实例只服务一次登录。
type loginServer struct {
	controller              controller
	ctx                     context.Context
	listener                net.Listener
	acceptsCallback         callbackURLAccepter
	loginURL                string
	actualAddr              string
	loginChallenge          string
	resultCh                chan loginServerResult
	finalCh                 chan bool
	submitOnce              sync.Once
	finalOnce               sync.Once
	finalPageWaiters        sync.WaitGroup
	server                  *http.Server
	serveErr                chan error
	cleanupOnce             sync.Once
	notifyFinal             func(bool)
	cleanup                 func()
	writeErr                func(string, ...any)
	reportInvalidSubmission func(error)
	handlerCleanup          func()
}

func newLoginServer(a controller, ctx context.Context, listener net.Listener, acceptsCallback callbackURLAccepter, loginURL string) *loginServer {
	return &loginServer{
		controller: a, ctx: ctx, listener: listener, acceptsCallback: acceptsCallback, loginURL: loginURL,
		actualAddr: listener.Addr().String(), loginChallenge: pixivLoginChallenge(loginURL),
		resultCh: make(chan loginServerResult, 1), finalCh: make(chan bool, 1), serveErr: make(chan error, 1),
	}
}

func (s *loginServer) submit(result loginServerResult) {
	s.submitOnce.Do(func() { s.resultCh <- result })
}

// notifyFinal 先通知最终页，再等待浏览器 handler 写完响应。
// 服务器生命周期由调用方 cleanup 负责，避免过早 Shutdown 打断最终页。
func (s *loginServer) notifyFinalPage(ok bool) {
	s.finalOnce.Do(func() {
		select {
		case s.finalCh <- ok:
		default:
		}
		s.finalPageWaiters.Wait()
	})
}

// waitFinalPage 假定调用方已在 submit 之前 finalPageWaiters.Add(1)，
// 避免 submit 放行主流程后与 Wait 竞态。
func (s *loginServer) waitFinalPage(w http.ResponseWriter, r *http.Request) {
	defer s.finalPageWaiters.Done()
	select {
	case ok := <-s.finalCh:
		writeLoginFinalPage(w, ok, i18n.AcceptLanguage(r.Header.Get("Accept-Language")))
	case <-r.Context().Done():
		writeLoginFinalPage(w, false, i18n.AcceptLanguage(r.Header.Get("Accept-Language")))
	case <-s.ctx.Done():
		writeLoginFinalPage(w, false, i18n.AcceptLanguage(r.Header.Get("Accept-Language")))
	}
}

func (s *loginServer) installHandler(ctx context.Context) {
	s.initializeErrors()
	if err := urlscheme.EnsurePersistentIfNeeded(ctx); err != nil {
		s.writeErr(s.controller.env.Text("login.console_0"), err)
	}
	cleanup, err := urlscheme.Install(ctx, "http://"+s.actualAddr+"/callback")
	if err != nil {
		s.writeErr(s.controller.env.Text("login.console_1"), err)
	} else if cleanup != nil {
		s.handlerCleanup = cleanup
		s.writeErr("%s", s.controller.env.Text("login.console_2"))
		s.writeErr("%s", s.controller.env.Text("login.console_3"))
	}
}

func (s *loginServer) initializeErrors() {
	var errOutMu sync.Mutex
	s.writeErr = func(format string, args ...any) {
		for i, arg := range args {
			if cause, ok := arg.(error); ok {
				args[i] = s.controller.env.Language.ErrorText(cause)
			}
		}
		errOutMu.Lock()
		defer errOutMu.Unlock()
		fmt.Fprintf(s.controller.env.Err, format, args...)
	}
	s.reportInvalidSubmission = func(err error) {
		if err != nil {
			s.writeErr(s.controller.env.Text("login.console_4"), err)
		}
	}
}

func (s *loginServer) serve(loginURL string, noOpen bool) {
	s.initializeErrors()
	openTerminalRelay := func(rawURL string) error {
		s.writeErr("%s", s.controller.env.Text("login.console_5"))
		return browser.OpenURL(rawURL)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		writeLoginForm(w, loginURL, i18n.AcceptLanguage(r.Header.Get("Accept-Language")))
	})
	mux.HandleFunc("/callback", s.handleCallback)
	mux.HandleFunc("/manual", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeLoginForm(w, loginURL, i18n.AcceptLanguage(r.Header.Get("Accept-Language")))
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			s.reportInvalidSubmission(err)
			writeLoginFinalPage(w, false, i18n.AcceptLanguage(r.Header.Get("Accept-Language")))
			return
		}
		input := r.Form.Get("login_result")
		result := classifyLoginInput(input, s.acceptsCallback, s.loginChallenge)
		if result.Relayed && result.Err == nil {
			// fallback 页面可能经 SSH 转发在另一台机器的浏览器中打开。relay 必须由
			// 当前请求的浏览器继续，不能在运行 CLI 的无 GUI 主机上调用 xdg-open。
			s.writeErr("%s", s.controller.env.Text("login.console_6"))
			w.Header().Set("Location", result.RelayURL)
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		s.reportInvalidSubmission(result.Err)
		if result.Err != nil {
			writeLoginFinalPage(w, false, i18n.AcceptLanguage(r.Header.Get("Accept-Language")))
			return
		}
		// 与 /callback 相同：Add 必须先于 submit，避免与 notifyFinal 的 Wait 竞态。
		s.finalPageWaiters.Add(1)
		s.submit(result.loginServerResult)
		s.waitFinalPage(w, r)
	})
	s.server = &http.Server{Handler: mux}
	go func() {
		err := s.server.Serve(s.listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		s.serveErr <- err
	}()
	s.cleanup = func() {
		s.cleanupOnce.Do(func() {
			if s.handlerCleanup != nil {
				s.handlerCleanup()
			}
			_ = s.server.Shutdown(context.Background())
		})
	}
	s.writeLoginInstructions(loginURL, noOpen)
	enableTerminalFallback := s.controller.env.CanPrompt()
	if !noOpen {
		if err := browser.OpenURL(loginURL); err != nil {
			s.writeErr(s.controller.env.Text("login.console_7"), err)
		}
	}
	if enableTerminalFallback {
		go s.promptForLoginInput(openTerminalRelay)
	}
}

func (s *loginServer) writeLoginInstructions(loginURL string, noOpen bool) {
	s.writeErr(s.controller.env.Text("login.console_8"), loginURL)
	if noOpen {
		s.writeErr("%s", s.controller.env.Text("login.console_9"))
		if tunnelCommand, tunnelErr := loginSSHTunnelCommand(s.actualAddr); tunnelErr != nil {
			s.writeErr("%s", s.controller.env.Text("login.console_10"))
		} else {
			s.writeErr(s.controller.env.Text("login.console_11"), tunnelCommand)
		}
		s.writeErr("%s", s.controller.env.Text("login.console_12"))
	} else {
		s.writeErr("%s", s.controller.env.Text("login.console_13"))
	}
	s.writeErr(s.controller.env.Text("login.console_14"), s.actualAddr)
}

func (s *loginServer) promptForLoginInput(openTerminalRelay func(string) error) {
	for {
		input, err := s.controller.env.PromptInput(s.controller.env.Text("prompt.paste_the_returned_pixiv_sign_in_address_relay"), "")
		if err != nil {
			return
		}
		result := loginInputFromText(input, s.acceptsCallback, s.loginChallenge, openTerminalRelay)
		s.reportInvalidSubmission(result.Err)
		if result.Relayed {
			continue
		}
		if result.Err == nil {
			s.submit(result.loginServerResult)
			return
		}
	}
}

func (s *loginServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.RawQuery == "" {
		// macOS helper 将 callback 放入 fragment，避免授权码进入 loopback GET 请求和浏览器历史。
		// 浏览器脚本会先清空 fragment，再通过本地 POST 提交给 /manual 并等待最终页。
		writeLoginCallbackRelayPage(w, i18n.AcceptLanguage(r.Header.Get("Accept-Language")))
		return
	}
	result := loginCodeFromInput(callbackURLFromRequest(r), s.acceptsCallback)
	s.reportInvalidSubmission(result.Err)
	if result.Err != nil {
		// 输入校验失败可立即回失败页；不泄露敏感细节。
		writeLoginFinalPage(w, false, i18n.AcceptLanguage(r.Header.Get("Accept-Language")))
		return
	}
	// 先登记最终页 waiter，再 submit 放行主流程，保证 notifyFinal.Wait 可见该 waiter。
	s.finalPageWaiters.Add(1)
	s.submit(result)
	// 等 OAuth 真正完成后再返回最终成功/失败页。
	s.waitFinalPage(w, r)
}

// loginSSHTunnelCommand 只使用已绑定的 loopback listener 地址生成提示，不接受外部输入。
func loginSSHTunnelCommand(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", i18n.Errorf("error.parse_login_listener_address", err)
	}
	if host == "" || port == "" {
		return "", i18n.Errorf("error.login_listener_address_is_incomplete")
	}
	return fmt.Sprintf("ssh -N -L %s:%s:%s USER@SERVER", port, host, port), nil
}

// writeLoginForm 渲染包含 Pixiv 登录链接和手动回填表单的页面。
func writeLoginForm(w http.ResponseWriter, loginURL string, locale i18n.Locale) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Language", locale.String())
	if err := loginrelay.WriteManual(w, loginURL, locale); err != nil {
		http.Error(w, locale.Text("login.render_failed"), http.StatusInternalServerError)
	}
}

// writeLoginCallbackRelayPage 将 helper 传来的 fragment 安全地转为本地 POST。
// fragment 不会随 GET 发送给 loopback，脚本会在提交前将其从浏览器地址和历史记录中清除。
func writeLoginCallbackRelayPage(w http.ResponseWriter, locale i18n.Locale) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Language", locale.String())
	if err := loginrelay.WriteCallbackRelay(w, locale); err != nil {
		http.Error(w, locale.Text("login.render_failed"), http.StatusInternalServerError)
	}
}

// writeLoginFinalPage 在 OAuth 真正完成后返回最终页。
// 成功/失败标题与正文均居中；失败页使用固定文案，不回显敏感原因。
func writeLoginFinalPage(w http.ResponseWriter, ok bool, locale i18n.Locale) {
	loginrelay.WriteFinalPage(w, ok, locale)
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
		return loginInputResult{loginServerResult: loginServerResult{Err: i18n.Errorf("error.pixiv_authorization_relay_url_is_empty")}, Relayed: true}
	}
	if err := openRelay(result.RelayURL); err != nil {
		return loginInputResult{loginServerResult: loginServerResult{Err: i18n.Errorf("error.could_not_open_pixiv_authorization_relay_url", err)}, Relayed: true}
	}
	return loginInputResult{Relayed: true, RelayURL: result.RelayURL}
}

// loginCodeFromInput 对一条登录输入做最终分类：合法 browser/app callback 携带
// code；非法或与当前会话不匹配的输入只返回稳定错误类别。
func loginCodeFromInput(input string, acceptsCallback callbackURLAccepter) loginServerResult {
	input = strings.TrimSpace(input)
	if input == "" {
		return loginServerResult{Err: i18n.Errorf("error.sign_in_result_cannot_be_empty")}
	}
	if strings.Contains(input, "://") || strings.HasPrefix(input, "/") {
		parsed, err := url.Parse(input)
		if err != nil {
			return loginServerResult{Err: i18n.Errorf("error.invalid_sign_in_address")}
		}
		if parsed.RawQuery != "" {
			if acceptsCallback == nil || !acceptsCallback(input) {
				return loginServerResult{Err: i18n.Errorf("error.sign_in_address_does_not_match_this_login")}
			}
			return loginServerResult{Code: input}
		}
		return loginServerResult{Err: i18n.Errorf("error.sign_in_address_did_not_include_required_details")}
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
		return i18n.Errorf("error.addr_cannot_be_empty")
	}
	if addr == defaultLoginAddr {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return i18n.Errorf("error.invalid_addr", addr, err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return i18n.Errorf("error.addr_must_bind_to_a_loopback_address_got", addr)
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
	if !strings.EqualFold(target.Scheme, "https") || !strings.EqualFold(target.Host, "app-api.pixiv.net") || target.Path != "/web/v1/users/auth/pixiv/start" {
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
		return "", true, i18n.Errorf("error.invalid_pixiv_authorization_relay_url")
	}
	if !pixivAuthStartMatchesChallenge(returnTo, expectedChallenge) {
		return "", true, i18n.Errorf("error.pixiv_authorization_relay_url_does_not_match_this")
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
