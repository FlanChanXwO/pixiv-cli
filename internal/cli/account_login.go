package cli

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/loginhelper"
	publicpixiv "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

const (
	defaultLoginAddr = "127.0.0.1:0"
	loginPageTitle   = "pixiv-cli"
)

var (
	loginHooksMu          sync.RWMutex
	openBrowser           = defaultOpenBrowser
	installURLSchemeRelay = loginhelper.Install
	ensureURLSchemeRelay  = loginhelper.EnsurePersistent
)

type loginServerResult struct {
	code string
	err  error
}

type loginInputResult struct {
	loginServerResult
	relayed  bool
	relayURL string
}

type callbackURLAccepter = func(string) bool
type urlSchemeRelayInstaller func(context.Context, string) (func(), error)
type urlSchemeRelayEnsurer func(context.Context) error

type accountLoginOptions struct {
	proxyOptions
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

func (a app) newAccountLoginCommand() *cobra.Command {
	opts := accountLoginOptions{addr: defaultLoginAddr}
	cmd := &cobra.Command{
		Use:     "login",
		Short:   "Login with the Pixiv browser OAuth flow",
		Example: "pixiv auth login --use",
		Args:    requireExactArgs(0, "pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--relay-public-url URL] [--relay-listen-addr ADDR]"),
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
	a.bindProxyFlags(cmd, &opts.proxyOptions)
	return cmd
}

func (a app) accountLogin(cmd *cobra.Command, opts accountLoginOptions) error {
	proxyOverride, err := proxyOverrideFromFlags(cmd, opts.proxyOptions)
	if err != nil {
		return err
	}
	services := a.services()
	cfg, err := services.Login.LoadRuntime()
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

	loginFlow, err := services.Login.Start(application.SDKClientRequest{HTTPSProxyOverride: proxyOverride})
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
		callbackOrCode, notifyFinal, cleanupLoginServer, err = a.waitForRelayLoginCode(ctx, relayOptions, loginFlow.AcceptsCallbackURL, loginURL)
	} else {
		browserOpener, schemeRelayInstaller, schemeRelayEnsurer := currentLoginHooks()
		callbackOrCode, notifyFinal, cleanupLoginServer, err = a.waitForLoginCode(ctx, opts.addr, loginFlow.AcceptsCallbackURL, loginURL, opts.noOpen, browserOpener, schemeRelayInstaller, schemeRelayEnsurer)
	}
	if err != nil {
		return err
	}
	defer cleanupLoginServer()

	result, err := services.Login.Complete(ctx, loginFlow, application.LoginCompleteRequest{CallbackOrCode: callbackOrCode, UseAfterLogin: opts.useAfterLogin})
	if err != nil {
		// OAuth 真正失败后才向浏览器回最终失败页；页面不回显敏感原因。
		notifyFinal(false)
		return err
	}
	notifyFinal(true)

	out := accountOutFromResult(result)
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

func (a app) waitForLoginCode(ctx context.Context, addr string, acceptsCallback callbackURLAccepter, loginURL string, noOpen bool, browserOpener func(string) error, schemeRelayInstaller urlSchemeRelayInstaller, schemeRelayEnsurer urlSchemeRelayEnsurer) (code string, notifyFinal func(bool), cleanup func(), err error) {
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
		return browserOpener(rawURL)
	}
	if !noOpen {
		if schemeRelayEnsurer != nil {
			if err := schemeRelayEnsurer(ctx); err != nil {
				writeErr("warning: persistent pixiv:// callback handler is unavailable: %v\n", err)
			}
		}
		cleanup, err := schemeRelayInstaller(ctx, "http://"+actualAddr+"/callback")
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
		reportInvalidSubmission(result.err)
		if result.err != nil {
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
		input := r.Form.Get("code")
		if input == "" {
			input = r.Form.Get("callback_url")
		}
		result := classifyLoginInput(input, acceptsCallback, loginChallenge)
		if result.relayed && result.err == nil {
			// fallback 页面可能经 SSH 转发在另一台机器的浏览器中打开。relay 必须由
			// 当前请求的浏览器继续，不能在运行 CLI 的无 GUI 主机上调用 xdg-open。
			writeErr("Detected Pixiv authorization relay page; continuing it in the current browser.\n")
			w.Header().Set("Location", result.relayURL)
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		reportInvalidSubmission(result.err)
		if result.err != nil {
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
	} else {
		writeErr("Complete the official Pixiv authorization in your browser; the loopback callback or manual page will receive the result.\n")
	}
	writeErr("Manual fallback page: http://%s/\n", actualAddr)

	enableTerminalFallback := canPrompt(a)
	if !noOpen {
		if err := browserOpener(loginURL); err != nil {
			writeErr("warning: could not open browser: %v\n", err)
		}
	}
	if enableTerminalFallback {
		go func() {
			for {
				input, err := promptInput(a, "Paste callback URL, Pixiv relay URL, or authorization code", "")
				if err != nil {
					return
				}
				result := loginInputFromText(input, acceptsCallback, loginChallenge, openTerminalRelay)
				reportInvalidSubmission(result.err)
				if result.relayed {
					continue
				}
				if result.err == nil {
					submit(result.loginServerResult)
					return
				}
			}
		}()
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			cleanup()
			return "", notifyFinal, cleanup, result.err
		}
		// 成功拿到 code 后保持服务器存活，直到调用方 notifyFinal + cleanup。
		return result.code, notifyFinal, cleanup, nil
	case err := <-serveErr:
		cleanup()
		if err != nil {
			return "", notifyFinal, cleanup, err
		}
		return "", notifyFinal, cleanup, errors.New("login server stopped before receiving authorization code")
	case <-ctx.Done():
		cleanup()
		return "", notifyFinal, cleanup, ctx.Err()
	}
}

func writeLoginForm(w http.ResponseWriter, loginURL string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>%s</title></head><body>
<p>Open <a href="%s">Pixiv login</a>, then paste a callback URL, pixiv:// URL, Pixiv relay URL, or raw code.</p>
<form method="post" action="/manual">
<input name="code" autofocus>
<button type="submit">Submit</button>
</form>
</body></html>`, loginPageTitle, html.EscapeString(loginURL))
}

// writeLoginCallbackRelayPage 将 helper 传来的 fragment 安全地转为本地 POST。
// fragment 不会随 GET 发送给 loopback，脚本会在提交前将其从浏览器地址和历史记录中清除。
func writeLoginCallbackRelayPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>pixiv-cli</title>
<style>
html,body{height:100%;margin:0}
body{display:flex;align-items:center;justify-content:center;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f7f7f8;color:#222}
.card{text-align:center;padding:2rem 2.5rem;max-width:28rem}
h1{margin:0 0 .75rem;font-size:1.75rem;font-weight:600}
p{margin:0;line-height:1.6;color:#555}
</style></head>
<body><div class="card"><h1>Completing login...</h1><p>Please keep this page open.</p></div>
<script>
(() => {
  const callbackURL = window.location.hash.slice(1);
  window.history.replaceState(null, "", window.location.pathname);
  if (!callbackURL) {
    document.title = "pixiv-cli";
    document.querySelector(".card").innerHTML = "<h1>Login failed</h1><p>Login could not be completed. Return to the terminal to view details or try again.</p>";
    return;
  }
  const form = document.createElement("form");
  form.method = "post";
  form.action = "/manual";
  const input = document.createElement("input");
  input.type = "hidden";
  input.name = "code";
  input.value = callbackURL;
  form.appendChild(input);
  document.body.appendChild(form);
  form.submit();
})();
</script></body></html>`)
}

// writeLoginFinalPage 在 OAuth 真正完成后返回最终页。
// 成功/失败标题与正文均居中；失败页使用固定文案，不回显敏感原因。
func writeLoginFinalPage(w http.ResponseWriter, ok bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
	}
	title := "Login successful"
	body := "Login completed. You can close this page and return to the terminal."
	if !ok {
		title = "Login failed"
		body = "Login could not be completed. Return to the terminal to view details or try again."
	}
	_, _ = io.WriteString(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>`+loginPageTitle+`</title>
<style>
html,body{height:100%;margin:0}
body{display:flex;align-items:center;justify-content:center;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f7f7f8;color:#222}
.card{text-align:center;padding:2rem 2.5rem;max-width:28rem}
h1{margin:0 0 .75rem;font-size:1.75rem;font-weight:600}
p{margin:0;line-height:1.6;color:#555}
</style></head>
<body><div class="card"><h1>`+title+`</h1><p>`+body+`</p></div></body></html>`)
}

// classifyLoginInput 只解析并校验用户提交；HTTP fallback 可以让原浏览器继续 relay，
// TTY fallback 则可选择调用本地 browser opener。两者不能混淆，否则 SSH 远端会错误地
// 尝试在无 GUI 服务器上打开 relay。
func classifyLoginInput(input string, acceptsCallback callbackURLAccepter, expectedChallenge string) loginInputResult {
	if returnTo, ok, err := pixivPostRedirectRelayURL(input, expectedChallenge); ok {
		if err != nil {
			return loginInputResult{loginServerResult: loginServerResult{err: err}, relayed: true}
		}
		return loginInputResult{relayed: true, relayURL: returnTo}
	}
	return loginInputResult{loginServerResult: loginCodeFromInput(input, acceptsCallback)}
}

// loginInputFromText 保留终端回填的既有行为：用户显式粘贴已校验 relay URL 时，
// 仅调用当前机器的 browser opener 一次。HTTP fallback 走 classifyLoginInput，
// 由提交表单的浏览器自行跳转。
func loginInputFromText(input string, acceptsCallback callbackURLAccepter, expectedChallenge string, openRelay func(string) error) loginInputResult {
	result := classifyLoginInput(input, acceptsCallback, expectedChallenge)
	if !result.relayed || result.err != nil {
		return result
	}
	if result.relayURL == "" {
		return loginInputResult{loginServerResult: loginServerResult{err: errors.New("Pixiv authorization relay URL is empty")}, relayed: true}
	}
	if openRelay == nil {
		return loginInputResult{loginServerResult: loginServerResult{err: errors.New("browser opener is not configured")}, relayed: true}
	}
	if err := openRelay(result.relayURL); err != nil {
		return loginInputResult{loginServerResult: loginServerResult{err: fmt.Errorf("could not open Pixiv authorization relay URL: %w", err)}, relayed: true}
	}
	return loginInputResult{relayed: true, relayURL: result.relayURL}
}

func loginCodeFromInput(input string, acceptsCallback callbackURLAccepter) loginServerResult {
	input = strings.TrimSpace(input)
	if input == "" {
		return loginServerResult{err: errors.New("authorization code cannot be empty")}
	}
	if strings.Contains(input, "://") || strings.HasPrefix(input, "/") {
		parsed, err := url.Parse(input)
		if err != nil {
			return loginServerResult{err: errors.New("invalid callback URL")}
		}
		if parsed.RawQuery != "" {
			if acceptsCallback == nil || !acceptsCallback(input) {
				return loginServerResult{err: errors.New("callback URL does not match this login session")}
			}
			return loginServerResult{code: input}
		}
		return loginServerResult{err: errors.New("callback URL did not include query parameters")}
	}
	return loginServerResult{code: input}
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

func isBrowserCallbackURL(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	if strings.EqualFold(parsed.Scheme, "pixiv") && strings.EqualFold(parsed.Host, "account") && parsed.Path == "/login" {
		return true
	}
	return publicpixiv.IsOfficialOAuthCallbackURL(parsed.String())
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

func currentLoginHooks() (func(string) error, urlSchemeRelayInstaller, urlSchemeRelayEnsurer) {
	loginHooksMu.RLock()
	defer loginHooksMu.RUnlock()
	return openBrowser, installURLSchemeRelay, ensureURLSchemeRelay
}

func setOpenBrowserForTest(opener func(string) error) func() {
	loginHooksMu.Lock()
	old := openBrowser
	openBrowser = opener
	loginHooksMu.Unlock()
	return func() {
		loginHooksMu.Lock()
		openBrowser = old
		loginHooksMu.Unlock()
	}
}

// setURLSchemeRelayInstallerForTest 替换 URL scheme 安装器，供不应调用 macOS 系统命令的测试使用。
// 值与其余登录 hook 在同一锁下快照，避免异步 watcher 或后续测试观察到部分设置。
func setURLSchemeRelayInstallerForTest(installer urlSchemeRelayInstaller) func() {
	loginHooksMu.Lock()
	old := installURLSchemeRelay
	oldEnsure := ensureURLSchemeRelay
	installURLSchemeRelay = installer
	// 安装器被测试替身替换时，不能触发真实 LaunchServices/registry/XDG 修改；
	// 生产路径仍使用 default ensurer 建立持久按需 handler。
	ensureURLSchemeRelay = func(context.Context) error { return nil }
	loginHooksMu.Unlock()
	return func() {
		loginHooksMu.Lock()
		installURLSchemeRelay = old
		ensureURLSchemeRelay = oldEnsure
		loginHooksMu.Unlock()
	}
}

func defaultOpenBrowser(rawURL string) error {
	return browser.OpenURL(rawURL)
}

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
	if !publicpixiv.IsOfficialOAuthStartURL(target.String()) {
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

func pixivAuthStartMatchesChallenge(rawURL, expectedChallenge string) bool {
	if expectedChallenge == "" {
		return true
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return parsed.Query().Get("code_challenge") == expectedChallenge
}
