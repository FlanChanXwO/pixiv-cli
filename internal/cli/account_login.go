package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
	publicpixiv "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"
)

const defaultLoginAddr = "127.0.0.1:0"
const managedBrowserDebugStartup = 15 * time.Second
const pixivURLHandlerBundleID = "com.flanchan.pixiv-cli.url-handler"

var (
	loginHooksMu          sync.RWMutex
	loginOAuthBase        = pixiv.DefaultOAuthBase
	openBrowser           = defaultOpenBrowser
	watchBrowserLoginCode = defaultWatchBrowserLoginCode
	captureManagedBrowser = defaultCaptureManagedBrowser
	installURLSchemeRelay = installPixivURLSchemeRelay
	runAppleScript        = defaultRunAppleScript
)

type loginServerResult struct {
	code string
	err  error
}

type loginInputResult struct {
	loginServerResult
	relayed bool
}

type browserCodeWatcher func(context.Context, string, string, map[string]struct{}, func(string) error, func(loginServerResult), func(error))
type managedBrowserCapturer func(context.Context, string, string, func(loginServerResult), func(error)) (bool, func(), error)
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

	loginFlow, err := services.Login.Start()
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

	oauthBase, browserOpener, browserWatcher, schemeRelayInstaller := currentLoginHooks()
	loginURL := pixivLoginURL(loginFlow.Challenge, loginFlow.State)
	code, err := a.waitForLoginCode(ctx, opts.addr, loginFlow.State, loginURL, opts.noOpen, browserOpener, browserWatcher, schemeRelayInstaller)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.errOut, "Authorization code received; exchanging it for a refresh token.")

	result, err := services.Login.Complete(ctx, application.LoginCompleteRequest{
		Code:               code,
		Verifier:           loginFlow.Verifier,
		OAuthBase:          oauthBase,
		UseAfterLogin:      opts.useAfterLogin,
		HTTPSProxyOverride: proxyOverride,
	})
	if err != nil {
		return err
	}

	out := accountOutFromResult(result)
	if opts.jsonOut {
		return a.printJSON(out)
	}
	fmt.Fprintf(a.out, "account uid:%d saved\n", out.UserID)
	if out.Default {
		fmt.Fprintf(a.out, "default uid: %d\n", out.UserID)
	}
	if out.Username != "" {
		fmt.Fprintf(a.out, "username:%s\n", out.Username)
	}
	return nil
}

func (a app) waitForLoginCode(ctx context.Context, addr, state, loginURL string, noOpen bool, browserOpener func(string) error, browserWatcher browserCodeWatcher, schemeRelayInstaller urlSchemeRelayInstaller) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	watchCtx, stopWatching := context.WithCancel(ctx)
	var browserWatcherDone <-chan struct{}
	defer func() { cancelAndJoinBrowserWatcher(stopWatching, browserWatcherDone, nil) }()
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
	observeRelay := func(string) error {
		writeErr("Detected Pixiv authorization relay page; waiting for pixiv:// callback handoff.\n")
		return nil
	}
	var schemeRelayActive bool
	if !noOpen {
		cleanup, err := schemeRelayInstaller(ctx, "http://"+actualAddr+"/manual")
		if err != nil {
			writeErr("warning: pixiv:// callback handler is unavailable: %v\n", err)
		} else if cleanup != nil {
			schemeRelayActive = true
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
		result := loginInputFromText(input, state, loginChallenge, openManualRelay)
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
	} else if runtime.GOOS == "darwin" && browserWatcher != nil {
		writeErr("Watching supported browser history/session state for the Pixiv callback URL.\n")
	} else {
		writeErr("Automatic browser callback detection is unavailable here; use the manual fallback page or terminal prompt.\n")
	}
	writeErr("Manual fallback page: http://%s/\n", actualAddr)

	enableTerminalFallback := canPrompt(a)
	if !noOpen {
		initialSeen := currentBrowserCallbackURLSet(ctx)
		if browserWatcher != nil {
			done := make(chan struct{})
			browserWatcherDone = done
			go func() {
				defer close(done)
				browserWatcher(watchCtx, state, loginChallenge, initialSeen, observeRelay, submit, reportInvalidSubmission)
			}()
		}
		managedOpened := false
		if !schemeRelayActive && captureManagedBrowser != nil {
			var managedStop func()
			var err error
			managedOpened, managedStop, err = captureManagedBrowser(watchCtx, loginURL, state, submit, reportInvalidSubmission)
			if managedStop != nil {
				defer managedStop()
			}
			if err != nil {
				writeErr("warning: managed browser capture is unavailable: %v\n", err)
			}
		}
		if managedOpened {
			writeErr("Opened a managed browser window for Pixiv login; waiting for pixiv:// callback capture.\n")
		} else if err := browserOpener(loginURL); err != nil {
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
				result := loginInputFromText(input, state, loginChallenge, openManualRelay)
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

// cancelAndJoinBrowserWatcher 固化登录 watcher 的生命周期：先取消，再等待其退出。
// beforeJoin 仅为确定性的内部测试接缝；生产调用始终传 nil。
func cancelAndJoinBrowserWatcher(stop func(), done <-chan struct{}, beforeJoin func()) {
	stop()
	if done == nil {
		return
	}
	if beforeJoin != nil {
		beforeJoin()
	}
	// watcher 的契约是响应 watchCtx；这里不引入额外超时，真实阻塞应当显式暴露。
	<-done
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
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, _ = io.WriteString(w, "authorization code received; return to the CLI\n")
}

func writeLoginRelayResult(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "authorization relay opened; continue in the browser or paste the final callback/code\n")
}

func loginInputFromText(input, expectedState, expectedChallenge string, openRelay func(string) error) loginInputResult {
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
	return loginInputResult{loginServerResult: loginCodeFromInput(input, expectedState)}
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
			return loginCodeFromValues(parsed.Query(), expectedState, loginURLRequiresState(parsed))
		}
		return loginServerResult{err: errors.New("callback URL did not include query parameters")}
	}
	return loginServerResult{code: input}
}

func loginURLRequiresState(parsed *url.URL) bool {
	if parsed == nil {
		return true
	}
	// Pixiv 的 App OAuth 跳转会丢掉 state；只对官方回调形态放宽，避免任意 URL 绕过校验。
	if strings.EqualFold(parsed.Scheme, "pixiv") && strings.EqualFold(parsed.Host, "account") && parsed.Path == "/login" {
		return false
	}
	if publicpixiv.IsOfficialOAuthCallbackURL(parsed.String()) {
		return false
	}
	return true
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
	return publicpixiv.BuildLoginAuthorizationURL(challenge, state)
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

func currentLoginHooks() (string, func(string) error, browserCodeWatcher, urlSchemeRelayInstaller) {
	loginHooksMu.RLock()
	defer loginHooksMu.RUnlock()
	return loginOAuthBase, openBrowser, watchBrowserLoginCode, installURLSchemeRelay
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
	oldCapture := captureManagedBrowser
	openBrowser = opener
	captureManagedBrowser = nil
	loginHooksMu.Unlock()
	return func() {
		loginHooksMu.Lock()
		openBrowser = old
		captureManagedBrowser = oldCapture
		loginHooksMu.Unlock()
	}
}

func setBrowserCodeWatcherForTest(watcher browserCodeWatcher) func() {
	loginHooksMu.Lock()
	old := watchBrowserLoginCode
	watchBrowserLoginCode = watcher
	loginHooksMu.Unlock()
	return func() {
		loginHooksMu.Lock()
		watchBrowserLoginCode = old
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

func installPixivURLSchemeRelay(ctx context.Context, manualURL string) (func(), error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("pixiv:// callback handler is only supported on macOS")
	}
	if _, err := exec.LookPath("swiftc"); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("swift"); err != nil {
		return nil, err
	}
	appPath, err := pixivURLHandlerAppPath()
	if err != nil {
		return nil, err
	}
	if err := ensurePixivURLHandlerApp(ctx, appPath); err != nil {
		return nil, err
	}
	endpointPath, err := pixivURLHandlerEndpointPath()
	if err != nil {
		return nil, err
	}
	if err := files.WritePrivateFile(endpointPath, []byte(strings.TrimSpace(manualURL)+"\n"), constants.PrivateFileMode); err != nil {
		return nil, err
	}
	previous, _ := defaultURLSchemeHandler(ctx, "pixiv")
	if err := registerURLHandlerApp(ctx, appPath); err != nil {
		return nil, err
	}
	if err := setDefaultURLSchemeHandler(ctx, "pixiv", pixivURLHandlerBundleID); err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = os.Remove(endpointPath)
		if previous != "" && previous != pixivURLHandlerBundleID {
			_ = setDefaultURLSchemeHandler(context.Background(), "pixiv", previous)
		}
	}
	return cleanup, nil
}

func pixivURLHandlerAppPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Applications", "PixivCLIURLHandler.app"), nil
}

func pixivURLHandlerEndpointPath() (string, error) {
	dir, err := files.UserConfigSubdir(constants.AppConfigDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "url-handler-endpoint"), nil
}

func ensurePixivURLHandlerApp(ctx context.Context, appPath string) error {
	executablePath := filepath.Join(appPath, "Contents", "MacOS", "PixivCLIURLHandler")
	infoPath := filepath.Join(appPath, "Contents", "Info.plist")
	if fileExists(executablePath) && fileExists(infoPath) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(executablePath), constants.PrivateDirMode); err != nil {
		return err
	}
	sourcePath := filepath.Join(os.TempDir(), "pixiv-cli-url-handler.swift")
	if err := os.WriteFile(sourcePath, []byte(pixivURLHandlerSwiftSource), constants.PrivateFileMode); err != nil {
		return err
	}
	defer os.Remove(sourcePath)
	cmd := exec.CommandContext(ctx, "swiftc", sourcePath, "-o", executablePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compile pixiv:// callback helper: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if err := os.WriteFile(infoPath, []byte(pixivURLHandlerInfoPlist), constants.PrivateFileMode); err != nil {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func registerURLHandlerApp(ctx context.Context, appPath string) error {
	lsregister := "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	cmd := exec.CommandContext(ctx, lsregister, "-f", appPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("register pixiv:// callback helper: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func defaultURLSchemeHandler(ctx context.Context, scheme string) (string, error) {
	script := fmt.Sprintf(`import Foundation; import CoreServices; let scheme=%q as NSString; if let h = LSCopyDefaultHandlerForURLScheme(scheme)?.takeRetainedValue() { print(h as String) }`, scheme)
	out, err := exec.CommandContext(ctx, "swift", "-e", script).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func setDefaultURLSchemeHandler(ctx context.Context, scheme, bundleID string) error {
	script := fmt.Sprintf(`import Foundation; import CoreServices; let scheme=%q as NSString; let handler=%q as NSString; let status = LSSetDefaultHandlerForURLScheme(scheme, handler); if status != 0 { exit(1) }`, scheme, bundleID)
	if out, err := exec.CommandContext(ctx, "swift", "-e", script).CombinedOutput(); err != nil {
		return fmt.Errorf("set %s:// callback handler: %w: %s", scheme, err, strings.TrimSpace(string(out)))
	}
	return nil
}

const pixivURLHandlerInfoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key><string>PixivCLIURLHandler</string>
  <key>CFBundleIdentifier</key><string>` + pixivURLHandlerBundleID + `</string>
  <key>CFBundleName</key><string>PixivCLIURLHandler</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSUIElement</key><true/>
  <key>CFBundleURLTypes</key>
  <array>
    <dict>
      <key>CFBundleURLName</key><string>Pixiv CLI OAuth Callback</string>
      <key>CFBundleURLSchemes</key>
      <array><string>pixiv</string></array>
    </dict>
  </array>
</dict>
</plist>
`

const pixivURLHandlerSwiftSource = `import Cocoa
import Foundation

final class AppDelegate: NSObject, NSApplicationDelegate {
    override init() {
        super.init()
        NSAppleEventManager.shared().setEventHandler(self, andSelector: #selector(handleGetURLEvent(_:withReplyEvent:)), forEventClass: AEEventClass(kInternetEventClass), andEventID: AEEventID(kAEGetURL))
    }

    @objc func handleGetURLEvent(_ event: NSAppleEventDescriptor, withReplyEvent replyEvent: NSAppleEventDescriptor) {
        guard let callbackURL = event.paramDescriptor(forKeyword: keyDirectObject)?.stringValue else {
            NSApp.terminate(nil)
            return
        }
        post(callbackURL)
        NSApp.terminate(nil)
    }

    private func post(_ callbackURL: String) {
        guard let endpointPath = endpointPath(),
              let endpoint = try? String(contentsOfFile: endpointPath, encoding: .utf8).trimmingCharacters(in: .whitespacesAndNewlines),
              let url = URL(string: endpoint),
              !endpoint.isEmpty else {
            return
        }
        var components = URLComponents()
        components.queryItems = [URLQueryItem(name: "code", value: callbackURL)]
        let body = components.percentEncodedQuery ?? ""
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        request.httpBody = body.data(using: .utf8)
        let semaphore = DispatchSemaphore(value: 0)
        URLSession.shared.dataTask(with: request) { _, _, _ in
            semaphore.signal()
        }.resume()
        _ = semaphore.wait(timeout: .now() + 10)
    }

    private func endpointPath() -> String? {
        guard let supportDir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first else {
            return nil
        }
        return supportDir.appendingPathComponent("pixiv/url-handler-endpoint").path
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.accessory)
app.run()
`

func defaultCaptureManagedBrowser(ctx context.Context, loginURL, expectedState string, submit func(loginServerResult), reportInvalid func(error)) (bool, func(), error) {
	browserPath := managedBrowserExecutable()
	if browserPath == "" {
		return false, nil, errors.New("supported Chromium/Edge browser executable was not found")
	}
	profileDir, err := managedBrowserProfileDir()
	if err != nil {
		return false, nil, err
	}
	if err := os.MkdirAll(profileDir, constants.PrivateDirMode); err != nil {
		return false, nil, err
	}
	if err := os.Chmod(profileDir, constants.PrivateDirMode); err != nil {
		return false, nil, err
	}
	port, err := freeTCPPort()
	if err != nil {
		return false, nil, err
	}
	cmd := exec.CommandContext(ctx, browserPath,
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-allow-origins=http://127.0.0.1",
		"--no-first-run",
		"--no-default-browser-check",
		"--new-window",
		"--user-data-dir="+profileDir,
		"about:blank",
	)
	if err := cmd.Start(); err != nil {
		return false, nil, err
	}
	stop := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}
	wsURL, err := openManagedBrowserTab(ctx, port, loginURL)
	if err != nil {
		stop()
		return false, nil, err
	}
	go watchManagedBrowserCDP(ctx, wsURL, expectedState, submit, reportInvalid)
	return true, stop, nil
}

func managedBrowserExecutable() string {
	for _, candidate := range managedBrowserExecutableCandidates() {
		if filepath.IsAbs(candidate) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}

func managedBrowserExecutableCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "windows":
		return []string{"msedge.exe", "chrome.exe", "chromium.exe"}
	default:
		return []string{"microsoft-edge", "microsoft-edge-stable", "google-chrome", "google-chrome-stable", "chromium", "chromium-browser"}
	}
}

func managedBrowserProfileDir() (string, error) {
	dir, err := files.UserConfigSubdir(constants.AppConfigDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth-browser-profile"), nil
}

func freeTCPPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func openManagedBrowserTab(ctx context.Context, port int, loginURL string) (string, error) {
	debugBase := fmt.Sprintf("http://127.0.0.1:%d", port)
	// 只限制本地子进程 DevTools 端口启动阶段；登录等待仍完全由用户配置的 auth timeout 控制。
	startupCtx := ctx
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		startupCtx, cancel = context.WithTimeout(ctx, managedBrowserDebugStartup)
		defer cancel()
	}
	client := http.Client{}
	for {
		req, err := http.NewRequestWithContext(startupCtx, http.MethodPut, debugBase+"/json/new?"+url.QueryEscape(loginURL), nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err == nil {
			var tab struct {
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&tab)
			_ = resp.Body.Close()
			if decodeErr == nil && tab.WebSocketDebuggerURL != "" {
				return tab.WebSocketDebuggerURL, nil
			}
		}
		select {
		case <-startupCtx.Done():
			return "", startupCtx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func watchManagedBrowserCDP(ctx context.Context, wsURL, expectedState string, submit func(loginServerResult), reportInvalid func(error)) {
	ws, err := websocket.Dial(wsURL, "", "http://127.0.0.1")
	if err != nil {
		reportInvalid(fmt.Errorf("could not attach to managed browser DevTools: %w", err))
		return
	}
	defer ws.Close()
	_ = websocket.JSON.Send(ws, map[string]any{"id": 1, "method": "Network.enable"})
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = ws.Close()
		case <-done:
		}
	}()
	defer close(done)
	for {
		var event map[string]any
		if err := websocket.JSON.Receive(ws, &event); err != nil {
			if ctx.Err() == nil {
				reportInvalid(fmt.Errorf("managed browser DevTools disconnected: %w", err))
			}
			return
		}
		result, ok := loginCodeFromCDPEvent(event, expectedState)
		if !ok {
			continue
		}
		if result.err != nil {
			reportInvalid(result.err)
			continue
		}
		submit(result)
		return
	}
}

func loginCodeFromCDPEvent(event map[string]any, expectedState string) (loginServerResult, bool) {
	if event == nil || event["method"] != "Network.requestWillBeSent" {
		return loginServerResult{}, false
	}
	params, _ := event["params"].(map[string]any)
	request, _ := params["request"].(map[string]any)
	rawURL, _ := request["url"].(string)
	if rawURL == "" {
		rawURL, _ = params["documentURL"].(string)
	}
	return loginCodeFromBrowserURL(rawURL, expectedState)
}

func defaultWatchBrowserLoginCode(ctx context.Context, expectedState, expectedChallenge string, initialSeen map[string]struct{}, openURL func(string) error, submit func(loginServerResult), reportInvalid func(error)) {
	if runtime.GOOS != "darwin" {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	seen := cloneStringSet(initialSeen)
	handleURLs := func(urls []string) bool {
		for _, rawURL := range urls {
			if returnTo, ok := pixivPostRedirectReturnTo(rawURL); ok {
				if !pixivAuthStartMatchesChallenge(returnTo, expectedChallenge) {
					continue
				}
				if _, ok := seen[rawURL]; ok {
					continue
				}
				seen[rawURL] = struct{}{}
				if err := openURL(rawURL); err != nil {
					reportInvalid(fmt.Errorf("could not open Pixiv authorization relay URL: %w", err))
				}
				continue
			}
			if _, ok := seen[rawURL]; ok {
				continue
			}
			seen[rawURL] = struct{}{}
			result, ok := loginCodeFromBrowserURL(rawURL, expectedState)
			if !ok {
				continue
			}
			if result.err != nil {
				reportInvalid(result.err)
				continue
			}
			submit(result)
			return true
		}
		return false
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if handleURLs(activeMacBrowserURLs(ctx)) {
				return
			}
			if handleURLs(callbackURLsFromChromiumStateFiles()) {
				return
			}
		}
	}
}

func currentBrowserLoginURLs(ctx context.Context) []string {
	seen := map[string]struct{}{}
	for _, rawURL := range activeMacBrowserURLs(ctx) {
		addBrowserLoginURL(seen, rawURL)
	}
	for _, rawURL := range callbackURLsFromChromiumStateFiles() {
		addBrowserLoginURL(seen, rawURL)
	}
	out := make([]string, 0, len(seen))
	for rawURL := range seen {
		out = append(out, rawURL)
	}
	sort.Strings(out)
	return out
}

func currentBrowserCallbackURLSet(ctx context.Context) map[string]struct{} {
	seen := map[string]struct{}{}
	for _, rawURL := range currentBrowserLoginURLs(ctx) {
		seen[rawURL] = struct{}{}
	}
	return seen
}

func cloneStringSet(values map[string]struct{}) map[string]struct{} {
	clone := map[string]struct{}{}
	for value := range values {
		clone[value] = struct{}{}
	}
	return clone
}

func addBrowserLoginURL(urls map[string]struct{}, rawURL string) {
	if _, ok := loginCodeFromBrowserURL(rawURL, ""); ok {
		urls[rawURL] = struct{}{}
		return
	}
	if _, ok := pixivPostRedirectReturnTo(rawURL); ok {
		urls[rawURL] = struct{}{}
	}
}

func loginCodeFromBrowserURL(rawURL, expectedState string) (loginServerResult, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.RawQuery == "" {
		return loginServerResult{}, false
	}
	if loginURLRequiresState(parsed) {
		return loginServerResult{}, false
	}
	return loginCodeFromValues(parsed.Query(), expectedState, false), true
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

func callbackURLsFromChromiumStateFiles() []string {
	files := chromiumStateFiles()
	seen := map[string]struct{}{}
	for _, filePath := range files {
		// 仅扫描浏览器状态文件中的 Pixiv 授权回调 URL，不读取 cookie、local storage 或 token。
		body, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		for _, rawURL := range callbackURLsFromBytes(body) {
			seen[rawURL] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for rawURL := range seen {
		out = append(out, rawURL)
	}
	sort.Strings(out)
	return out
}

func chromiumStateFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	patterns := []string{
		filepath.Join(home, "Library", "Application Support", "Microsoft Edge", "*", "Sessions", "*"),
		filepath.Join(home, "Library", "Application Support", "Microsoft Edge", "*", "History"),
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "*", "Sessions", "*"),
		filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "*", "History"),
		filepath.Join(home, "Library", "Application Support", "Chromium", "*", "Sessions", "*"),
		filepath.Join(home, "Library", "Application Support", "Chromium", "*", "History"),
	}
	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}
			files = append(files, match)
		}
	}
	sort.Strings(files)
	return files
}

func callbackURLsFromBytes(body []byte) []string {
	prefixes := [][]byte{
		[]byte(publicpixiv.OAuthCallbackURLPrefix()),
		[]byte("pixiv://account/login?"),
	}
	seen := map[string]struct{}{}
	for _, prefix := range prefixes {
		for searchStart := 0; searchStart < len(body); {
			relative := bytes.Index(body[searchStart:], prefix)
			if relative < 0 {
				break
			}
			start := searchStart + relative
			end := start
			for end < len(body) && isURLByte(body[end]) {
				end++
			}
			if end > start {
				seen[string(body[start:end])] = struct{}{}
			}
			searchStart = end
		}
	}
	out := make([]string, 0, len(seen))
	for rawURL := range seen {
		out = append(out, rawURL)
	}
	sort.Strings(out)
	return out
}

func isURLByte(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		strings.ContainsRune(":/?#[]@!$&'()*+,;=%-._~", rune(ch))
}

func activeMacBrowserURLs(ctx context.Context) []string {
	if urls, err := activeMacBrowserURLsViaBrowserScripting(ctx); err == nil && len(urls) > 0 {
		return urls
	}
	if rawURL, err := activeMacBrowserURLViaSystemEvents(ctx); err == nil && rawURL != "" {
		return []string{rawURL}
	}
	return nil
}

func activeMacBrowserURLsViaBrowserScripting(ctx context.Context) ([]string, error) {
	script := `
set browserURLs to ""
if application id "com.microsoft.edgemac" is running then
	try
		tell application id "com.microsoft.edgemac"
			repeat with browserWindow in windows
				repeat with browserTab in tabs of browserWindow
					set browserURL to URL of browserTab
					if browserURL is not missing value and browserURL is not "" then set browserURLs to browserURLs & browserURL & linefeed
				end repeat
			end repeat
		end tell
	end try
end if
if application id "com.google.Chrome" is running then
	try
		tell application id "com.google.Chrome"
			repeat with browserWindow in windows
				repeat with browserTab in tabs of browserWindow
					set browserURL to URL of browserTab
					if browserURL is not missing value and browserURL is not "" then set browserURLs to browserURLs & browserURL & linefeed
				end repeat
			end repeat
		end tell
	end try
end if
if application id "org.chromium.Chromium" is running then
	try
		tell application id "org.chromium.Chromium"
			repeat with browserWindow in windows
				repeat with browserTab in tabs of browserWindow
					set browserURL to URL of browserTab
					if browserURL is not missing value and browserURL is not "" then set browserURLs to browserURLs & browserURL & linefeed
				end repeat
			end repeat
		end tell
	end try
end if
if application id "com.apple.Safari" is running then
	try
		tell application id "com.apple.Safari"
			repeat with browserWindow in windows
				repeat with browserTab in tabs of browserWindow
					set browserURL to URL of browserTab
					if browserURL is not missing value and browserURL is not "" then set browserURLs to browserURLs & browserURL & linefeed
				end repeat
			end repeat
		end tell
	end try
end if
return browserURLs
`
	out, err := runAppleScript(ctx, script)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	urls := make([]string, 0, len(lines))
	for _, line := range lines {
		if rawURL := strings.TrimSpace(line); rawURL != "" {
			urls = append(urls, rawURL)
		}
	}
	return urls, nil
}

func activeMacBrowserURLViaSystemEvents(ctx context.Context) (string, error) {
	script := `
tell application "System Events"
	set browserNames to {"Microsoft Edge", "Google Chrome", "Chromium", "Safari"}
	repeat with browserName in browserNames
		if exists process (contents of browserName) then
			tell process (contents of browserName)
				if (count of windows) is 0 then
					set browserURL to ""
				else
					set browserURL to ""
					try
						set browserURL to value of text field 1 of group 1 of toolbar 1 of window 1
					end try
					if browserURL is "" then
						try
							set browserURL to value of text field 1 of toolbar 1 of window 1
						end try
					end if
					if browserURL is not "" then return browserURL
				end if
			end tell
		end if
	end repeat
end tell
return ""
`
	out, err := runAppleScript(ctx, script)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func defaultRunAppleScript(ctx context.Context, script string) (string, error) {
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
