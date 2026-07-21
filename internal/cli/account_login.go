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

const defaultLoginAddr = "127.0.0.1:0"

var (
	loginHooksMu          sync.RWMutex
	openBrowser           = defaultOpenBrowser
	installURLSchemeRelay = loginhelper.Install
)

type loginServerResult struct {
	code string
	err  error
}

type loginInputResult struct {
	loginServerResult
	relayed bool
}

type callbackURLAccepter = func(string) bool
type urlSchemeRelayInstaller func(context.Context, string) (func(), error)

type accountLoginOptions struct {
	proxyOptions
	jsonOut       bool
	noOpen        bool
	useAfterLogin bool
	addr          string
	timeout       time.Duration
}

func (a app) newAccountLoginCommand() *cobra.Command {
	opts := accountLoginOptions{addr: defaultLoginAddr}
	cmd := &cobra.Command{
		Use:     "login",
		Short:   "Login with the Pixiv browser OAuth flow",
		Example: "pixiv auth login --use",
		Args:    requireExactArgs(0, "pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountLogin(cmd, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	flags.BoolVar(&opts.noOpen, "no-open", false, "do not open the browser")
	flags.StringVar(&opts.addr, "addr", defaultLoginAddr, "local loopback listen address")
	flags.BoolVar(&opts.useAfterLogin, "use", false, "set as default account after login")
	flags.DurationVar(&opts.timeout, "timeout", 0, "maximum time to wait for login flow")
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
	if !cmd.Flags().Changed("timeout") {
		opts.timeout = cfg.LoginTimeout
	}
	if err := validateLoginAddr(opts.addr); err != nil {
		return err
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

	browserOpener, schemeRelayInstaller := currentLoginHooks()
	loginURL := loginFlow.AuthorizationURL
	callbackOrCode, notifyFinal, cleanupLoginServer, err := a.waitForLoginCode(ctx, opts.addr, loginFlow.AcceptsCallbackURL, loginURL, opts.noOpen, browserOpener, schemeRelayInstaller)
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
	// 成功提示前空一行，便于与前面的授权引导输出分隔。
	fmt.Fprintf(a.out, "\n登录成功（UID: %d）\n", out.UserID)
	return nil
}

func (a app) waitForLoginCode(ctx context.Context, addr string, acceptsCallback callbackURLAccepter, loginURL string, noOpen bool, browserOpener func(string) error, schemeRelayInstaller urlSchemeRelayInstaller) (code string, notifyFinal func(bool), cleanup func(), err error) {
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
	openManualRelay := func(rawURL string) error {
		writeErr("Detected Pixiv authorization relay page; opening Pixiv relay URL once.\n")
		return browserOpener(rawURL)
	}
	if !noOpen {
		cleanup, err := schemeRelayInstaller(ctx, "http://"+actualAddr+"/manual")
		if err != nil {
			writeErr("warning: pixiv:// callback handler is unavailable: %v\n", err)
		} else if cleanup != nil {
			defer cleanup()
			writeErr("Registered pixiv:// callback handler for this login attempt.\n")
			writeErr("After confirming Pixiv account, the browser may stay on a white Pixiv relay page; keep this terminal open for the result.\n")
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
		result := loginInputFromText(input, acceptsCallback, loginChallenge, openManualRelay)
		if result.relayed && result.err == nil {
			writeLoginRelayResult(w)
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
				result := loginInputFromText(input, acceptsCallback, loginChallenge, openManualRelay)
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
<html><body>
<p>Open <a href="%s">Pixiv login</a>, then paste a callback URL, pixiv:// URL, Pixiv relay URL, or raw code.</p>
<form method="post" action="/manual">
<input name="code" autofocus>
<button type="submit">Submit</button>
</form>
</body></html>`, html.EscapeString(loginURL))
}

// writeLoginFinalPage 在 OAuth 真正完成后返回最终页。
// 成功/失败标题与正文均居中；失败页使用固定文案，不回显敏感原因。
func writeLoginFinalPage(w http.ResponseWriter, ok bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
	}
	title := "登录成功"
	body := "登录已完成，可以关闭此页面并返回终端。"
	if !ok {
		title = "登录失败"
		body = "登录未能完成，请返回终端查看提示或重试。"
	}
	_, _ = io.WriteString(w, `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>`+title+`</title>
<style>
html,body{height:100%;margin:0}
body{display:flex;align-items:center;justify-content:center;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f7f7f8;color:#222}
.card{text-align:center;padding:2rem 2.5rem;max-width:28rem}
h1{margin:0 0 .75rem;font-size:1.75rem;font-weight:600}
p{margin:0;line-height:1.6;color:#555}
</style></head>
<body><div class="card"><h1>`+title+`</h1><p>`+body+`</p></div></body></html>`)
}

func writeLoginRelayResult(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "authorization relay opened; continue in the browser or paste the final callback/code\n")
}

func loginInputFromText(input string, acceptsCallback callbackURLAccepter, expectedChallenge string, openRelay func(string) error) loginInputResult {
	if returnTo, ok, err := pixivPostRedirectRelayURL(input, expectedChallenge); ok {
		if err != nil {
			return loginInputResult{loginServerResult: loginServerResult{err: err}, relayed: true}
		}
		if openRelay == nil {
			return loginInputResult{loginServerResult: loginServerResult{err: errors.New("browser opener is not configured")}, relayed: true}
		}
		if err := openRelay(returnTo); err != nil {
			return loginInputResult{loginServerResult: loginServerResult{err: fmt.Errorf("could not open Pixiv authorization relay URL: %w", err)}, relayed: true}
		}
		return loginInputResult{relayed: true}
	}
	return loginInputResult{loginServerResult: loginCodeFromInput(input, acceptsCallback)}
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

func currentLoginHooks() (func(string) error, urlSchemeRelayInstaller) {
	loginHooksMu.RLock()
	defer loginHooksMu.RUnlock()
	return openBrowser, installURLSchemeRelay
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
	installURLSchemeRelay = installer
	loginHooksMu.Unlock()
	return func() {
		loginHooksMu.Lock()
		installURLSchemeRelay = old
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
