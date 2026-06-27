package cmd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

const defaultLoginAddr = "127.0.0.1:0"

var (
	loginHooksMu   sync.RWMutex
	loginOAuthBase = pixiv.DefaultOAuthBase
	openBrowser    = defaultOpenBrowser
)

type loginServerResult struct {
	code string
	err  error
}

type accountLoginOptions struct {
	jsonOut    bool
	noOpen     bool
	useProfile bool
	addr       string
	timeout    time.Duration
}

func (a app) newAccountLoginCommand() *cobra.Command {
	opts := accountLoginOptions{addr: defaultLoginAddr}
	cmd := &cobra.Command{
		Use:     "login [NAME]",
		Short:   "Login with the Pixiv browser OAuth flow",
		Example: "pixiv account login main --use",
		Args:    requireMaxArgs(1, "pixiv account login [NAME] [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.accountLogin(cmd, args, opts)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&opts.jsonOut, "json", false, "print JSON")
	flags.BoolVar(&opts.noOpen, "no-open", false, "do not open the browser")
	flags.StringVar(&opts.addr, "addr", defaultLoginAddr, "local loopback listen address")
	flags.BoolVar(&opts.useProfile, "use", false, "set as default account after login")
	flags.DurationVar(&opts.timeout, "timeout", 0, "maximum time to wait for login flow")
	return cmd
}

func (a app) accountLogin(cmd *cobra.Command, args []string, opts accountLoginOptions) error {
	settings, err := loadSettingsState()
	if err != nil {
		return err
	}
	cfg, err := settings.runtime()
	if err != nil {
		return err
	}
	if !cmd.Flags().Changed("no-open") {
		opts.noOpen = !cfg.LoginOpenBrowser
	}
	if !cmd.Flags().Changed("use") {
		opts.useProfile = cfg.LoginUseAfterLogin
	}
	if !cmd.Flags().Changed("timeout") {
		opts.timeout = cfg.LoginTimeout
	}
	name, err := a.resolveAccountNameArg(args, "Account name")
	if err != nil {
		return err
	}
	if err := validateAccountName(name); err != nil {
		return err
	}
	if err := validateLoginAddr(opts.addr); err != nil {
		return err
	}

	verifier, challenge, err := generatePKCEPair()
	if err != nil {
		return err
	}
	state, err := randomURLToken(32)
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

	loginURL := pixivLoginURL(challenge, state)
	oauthBase, browserOpener := currentLoginHooks()
	code, err := a.waitForLoginCode(ctx, opts.addr, state, loginURL, opts.noOpen, browserOpener)
	if err != nil {
		return err
	}

	client, err := newLoginPixivClient(cfg, oauthBase)
	if err != nil {
		return err
	}
	token, err := client.ExchangeAuthorizationCode(ctx, code, verifier)
	if err != nil {
		return err
	}

	path, err := authFilePath()
	if err != nil {
		return err
	}
	store, err := loadAuthStore(path)
	if err != nil {
		return err
	}
	store.upsert(account{Name: name, RefreshToken: token.RefreshToken, UserID: token.UserID})
	if opts.useProfile || store.DefaultAccount == "" {
		store.DefaultAccount = name
	}
	if err := saveAuthStore(path, store); err != nil {
		return err
	}

	out := accountOut{Name: name, Default: store.DefaultAccount == name, UserID: token.UserID, HasToken: true}
	if opts.jsonOut {
		return a.printJSON(out)
	}
	fmt.Fprintf(a.out, "account %q saved\n", name)
	if out.Default {
		fmt.Fprintf(a.out, "default account: %s\n", name)
	}
	if out.UserID != 0 {
		fmt.Fprintf(a.out, "user_id:%d\n", out.UserID)
	}
	return nil
}

func (a app) waitForLoginCode(ctx context.Context, addr, state, loginURL string, noOpen bool, browserOpener func(string) error) (string, error) {
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

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		writeLoginForm(w, loginURL)
	})
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		result := loginCodeFromValues(r.URL.Query(), state, true)
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
		result := loginCodeFromInput(input, state)
		writeLoginResult(w, result.err)
		reportInvalidSubmission(result.err)
		if result.err == nil {
			submit(result)
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
	writeErr("Waiting for callback or manual code at http://%s/\n", actualAddr)

	enableTerminalFallback := noOpen && canPrompt(a)
	if !noOpen {
		if err := browserOpener(loginURL); err != nil {
			writeErr("warning: could not open browser: %v\n", err)
			enableTerminalFallback = canPrompt(a)
		}
	}
	if enableTerminalFallback {
		go func() {
			input, err := promptInput(a, "Paste callback URL or authorization code", "")
			if err != nil {
				return
			}
			result := loginCodeFromInput(input, state)
			reportInvalidSubmission(result.err)
			if result.err == nil {
				submit(result)
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
<p>Open <a href="%s">Pixiv login</a>, then paste a callback URL, pixiv:// URL, or raw code.</p>
<form method="post" action="/manual">
<input name="code" autofocus>
<button type="submit">Submit</button>
</form>
</body></html>`, html.EscapeString(loginURL))
}

func writeLoginResult(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, _ = io.WriteString(w, "authorization code received; return to the CLI\n")
}

func loginCodeFromInput(input, expectedState string) loginServerResult {
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
			return loginCodeFromValues(parsed.Query(), expectedState, true)
		}
		return loginServerResult{err: errors.New("callback URL did not include query parameters")}
	}
	return loginServerResult{code: input}
}

func loginCodeFromValues(values url.Values, expectedState string, requireState bool) loginServerResult {
	code := strings.TrimSpace(values.Get("code"))
	if code == "" {
		return loginServerResult{err: errors.New("callback did not include authorization code")}
	}
	state := strings.TrimSpace(values.Get("state"))
	if requireState && state == "" {
		return loginServerResult{err: errors.New("OAuth state is required")}
	}
	if state != "" && state != expectedState {
		return loginServerResult{err: errors.New("OAuth state mismatch")}
	}
	return loginServerResult{code: code}
}

func pixivLoginURL(challenge, state string) string {
	values := url.Values{}
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("client", "pixiv-android")
	values.Set("state", state)
	return pixiv.DefaultAPIBase + "/web/v1/login?" + values.Encode()
}

func generatePKCEPair() (verifier, challenge string, err error) {
	verifier, err = randomURLToken(64)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomURLToken(size int) (string, error) {
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
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

func currentLoginHooks() (string, func(string) error) {
	loginHooksMu.RLock()
	defer loginHooksMu.RUnlock()
	return loginOAuthBase, openBrowser
}

func setLoginOAuthBaseForTest(baseURL string) func() {
	loginHooksMu.Lock()
	old := loginOAuthBase
	loginOAuthBase = baseURL
	loginHooksMu.Unlock()
	return func() {
		loginHooksMu.Lock()
		loginOAuthBase = old
		loginHooksMu.Unlock()
	}
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

func newLoginPixivClient(cfg runtimeConfig, oauthBase string) (*pixiv.Client, error) {
	httpClient, err := newHTTPClient(cfg.HTTPSProxy)
	if err != nil {
		return nil, err
	}
	return pixiv.New("", pixiv.WithHTTPClient(httpClient), pixiv.WithBaseURLs("", oauthBase)), nil
}

func defaultOpenBrowser(rawURL string) error {
	return browser.OpenURL(rawURL)
}
