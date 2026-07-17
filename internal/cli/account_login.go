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
	callbackOrCode, err := a.waitForLoginCode(ctx, opts.addr, loginFlow.AcceptsCallbackURL, loginURL, opts.noOpen, browserOpener, schemeRelayInstaller)
	if err != nil {
		return err
	}

	result, err := services.Login.Complete(ctx, loginFlow, application.LoginCompleteRequest{CallbackOrCode: callbackOrCode, UseAfterLogin: opts.useAfterLogin})
	if err != nil {
		return err
	}

	out := accountOutFromResult(result)
	if opts.jsonOut {
		return a.printJSON(out)
	}
	fmt.Fprintf(a.out, "登录成功（UID: %d）\n", out.UserID)
	return nil
}

func (a app) waitForLoginCode(ctx context.Context, addr string, acceptsCallback callbackURLAccepter, loginURL string, noOpen bool, browserOpener func(string) error, schemeRelayInstaller urlSchemeRelayInstaller) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	actualAddr := ln.Addr().String()
	resultCh := make(chan loginServerResult, 1)
	var submitOnce sync.Once
	submit := func(result loginServerResult) {
		submitOnce.Do(func() { resultCh <- result })
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
		writeLoginResult(w, result.err)
		reportInvalidSubmission(result.err)
		if result.err == nil {
			submit(result)
		}
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
			result := loginServerResult{err: err}
			writeLoginResult(w, result.err)
			reportInvalidSubmission(result.err)
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
		writeLoginResult(w, result.err)
		reportInvalidSubmission(result.err)
		if result.err == nil {
			submit(result.loginServerResult)
		}
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
	defer func() { _ = server.Shutdown(context.Background()) }()

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
			return "", result.err
		}
		return result.code, nil
	case err := <-serveErr:
		if err != nil {
			return "", err
		}
		return "", errors.New("login server stopped before receiving authorization code")
	case <-ctx.Done():
		return "", ctx.Err()
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

func writeLoginResult(w http.ResponseWriter, err error) {
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>授权已收到</title></head>
<body><p>已收到授权，正在回到 CLI 完成登录。</p></body></html>`)
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
