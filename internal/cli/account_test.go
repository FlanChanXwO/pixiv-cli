package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv"
)

func TestAccountAddListUseRemove(t *testing.T) {
	path := testConfigPath(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "account", "add", "--token", "foo=bar; refresh_token=main%2Ftoken", "main"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("add main code=%d stderr=%s", code, stderr.String())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != defaultConfigFileMode {
		t.Fatalf("config mode = %v", got)
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if store.DefaultProfile != "main" || store.Accounts["main"].RefreshToken != "main/token" {
		t.Fatalf("store = %+v", store)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "account", "add", "--token", "other-token", "other"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("add other code=%d stderr=%s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "account", "use", "other"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("use code=%d stderr=%s", code, stderr.String())
	}
	store, err = loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if store.DefaultProfile != "other" {
		t.Fatalf("default profile = %q", store.DefaultProfile)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "account", "list", "--json"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("list code=%d stderr=%s", code, stderr.String())
	}
	var out accountListOut
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if out.DefaultProfile != "other" || len(out.Accounts) != 2 {
		t.Fatalf("list output = %+v", out)
	}
	if strings.Contains(stdout.String(), "other-token") || strings.Contains(stdout.String(), "main/token") {
		t.Fatalf("list leaked token: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"pixiv", "account", "remove", "other"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("remove code=%d stderr=%s", code, stderr.String())
	}
	store, err = loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if store.DefaultProfile != "main" {
		t.Fatalf("default after remove = %q", store.DefaultProfile)
	}
}

func TestAccountHelpListsLogin(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "account"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "login NAME") {
		t.Fatalf("account help did not list login command:\n%s", stdout.String())
	}
}

func TestAccountAddUsageShowsFlagBeforeName(t *testing.T) {
	testConfigPath(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "account", "add"}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero code")
	}
	if !strings.Contains(stderr.String(), "pixiv account add [--token TOKEN] NAME") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAccountLoginNoOpenStoresProfile(t *testing.T) {
	path := testConfigPath(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" {
			t.Fatalf("unexpected OAuth path %s", r.URL.Path)
		}
		if got := r.Header.Get("User-Agent"); got != pixiv.DefaultUserAgent {
			t.Fatalf("User-Agent = %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse OAuth form: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "authorization_code" {
			t.Fatalf("grant_type = %q", got)
		}
		if got := r.Form.Get("code"); got != "manual-code" {
			t.Fatalf("code = %q", got)
		}
		if got := r.Form.Get("code_verifier"); got == "" {
			t.Fatalf("code_verifier was empty")
		}
		if got := r.Form.Get("redirect_uri"); got != "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback" {
			t.Fatalf("redirect_uri = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-secret",
			"refresh_token": "refresh-secret",
			"user":          map[string]any{"id": "12345"},
		})
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "account", "login", "--addr", addr, "--no-open", "--timeout", "5s", "main"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	waitForLoginServer(t, addr)
	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {"manual-code"}})
	if err != nil {
		t.Fatalf("post manual code: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if code := run.waitWithin(t, 5*time.Second); code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if store.DefaultProfile != "main" {
		t.Fatalf("default profile = %q", store.DefaultProfile)
	}
	if got := store.Accounts["main"].RefreshToken; got != "refresh-secret" {
		t.Fatalf("stored refresh token = %q", got)
	}
	if got := store.Accounts["main"].UserID; got != 12345 {
		t.Fatalf("stored user_id = %d", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != defaultConfigFileMode {
		t.Fatalf("config mode = %v", got)
	}
	if strings.Contains(stdout.String(), "refresh-secret") || strings.Contains(stdout.String(), "access-secret") {
		t.Fatalf("stdout leaked token: %s", stdout.String())
	}
	if strings.Contains(stderr.String(), "refresh-secret") || strings.Contains(stderr.String(), "access-secret") {
		t.Fatalf("stderr leaked token: %s", stderr.String())
	}
}

func TestAccountLoginDirectCallbackWithStateStoresProfile(t *testing.T) {
	path := testConfigPath(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse OAuth form: %v", err)
		}
		if got := r.Form.Get("code"); got != "direct-code" {
			t.Fatalf("code = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "direct-refresh-secret",
			"user":          map[string]any{"id": "13579"},
		})
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "account", "login", "--addr", addr, "--no-open", "--timeout", "5s", "direct"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	loginURL := loginURLFromPage(t, waitForLoginServer(t, addr))
	state := loginState(t, loginURL)
	resp, err := http.Get("http://" + addr + "/callback?code=direct-code&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("send callback: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if code := run.waitWithin(t, 5*time.Second); code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if got := store.Accounts["direct"].RefreshToken; got != "direct-refresh-secret" {
		t.Fatalf("stored refresh token = %q", got)
	}
	if got := store.Accounts["direct"].UserID; got != 13579 {
		t.Fatalf("stored user_id = %d", got)
	}
}

func TestAccountLoginPastedHTTPSCallbackURLWithStateStoresProfile(t *testing.T) {
	path := testConfigPath(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse OAuth form: %v", err)
		}
		if got := r.Form.Get("code"); got != "https-url-code" {
			t.Fatalf("code = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "https-url-refresh-secret",
			"user":          map[string]any{"id": "22446"},
		})
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "account", "login", "--addr", addr, "--no-open", "--timeout", "5s", "https-url"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	state := loginState(t, loginURLFromPage(t, waitForLoginServer(t, addr)))
	callbackURL := "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=https-url-code&state=" + url.QueryEscape(state)
	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {callbackURL}})
	if err != nil {
		t.Fatalf("post HTTPS callback URL: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if code := run.waitWithin(t, 5*time.Second); code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if got := store.Accounts["https-url"].RefreshToken; got != "https-url-refresh-secret" {
		t.Fatalf("stored refresh token = %q", got)
	}
}

func TestAccountLoginPastedPixivURLWithStateStoresProfile(t *testing.T) {
	path := testConfigPath(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse OAuth form: %v", err)
		}
		if got := r.Form.Get("code"); got != "pixiv-url-code" {
			t.Fatalf("code = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "pixiv-url-refresh-secret",
			"user":          map[string]any{"id": "66880"},
		})
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "account", "login", "--addr", addr, "--no-open", "--timeout", "5s", "pixiv-url"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	state := loginState(t, loginURLFromPage(t, waitForLoginServer(t, addr)))
	callbackURL := "pixiv://auth/callback?code=pixiv-url-code&state=" + url.QueryEscape(state)
	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {callbackURL}})
	if err != nil {
		t.Fatalf("post pixiv callback URL: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if code := run.waitWithin(t, 5*time.Second); code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if got := store.Accounts["pixiv-url"].RefreshToken; got != "pixiv-url-refresh-secret" {
		t.Fatalf("stored refresh token = %q", got)
	}
}

func TestAccountLoginJSONDoesNotLeakToken(t *testing.T) {
	testConfigPath(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse OAuth form: %v", err)
		}
		if got := r.Form.Get("code"); got != "json-code" {
			t.Fatalf("code = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "json-access-secret",
			"refresh_token": "json-refresh-secret",
			"user":          map[string]any{"id": 67890},
		})
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "account", "login", "--json", "--addr", addr, "--no-open", "--timeout", "5s", "json"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	waitForLoginServer(t, addr)
	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {"json-code"}})
	if err != nil {
		t.Fatalf("post manual code: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if code := run.waitWithin(t, 5*time.Second); code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if _, ok := out["refresh_token"]; ok {
		t.Fatalf("JSON included refresh_token: %s", stdout.String())
	}
	if _, ok := out["access_token"]; ok {
		t.Fatalf("JSON included access_token: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "json-refresh-secret") || strings.Contains(stdout.String(), "json-access-secret") {
		t.Fatalf("stdout leaked token: %s", stdout.String())
	}
}

func TestAccountLoginRejectsNonLoopbackAddr(t *testing.T) {
	testConfigPath(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "account", "login", "--addr", "0.0.0.0:0", "--no-open", "main"}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero code")
	}
	if !strings.Contains(stderr.String(), "loopback") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAccountLoginRejectsStateMismatch(t *testing.T) {
	path := testConfigPath(t)
	addr := freeLoopbackAddr(t)
	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "account", "login", "--addr", addr, "--no-open", "--timeout", "300ms", "main"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	waitForLoginServer(t, addr)
	resp, err := http.Get("http://" + addr + "/callback?code=bad-code&state=wrong-state")
	if err != nil {
		t.Fatalf("send callback: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if code := run.waitWithin(t, 2*time.Second); code == 0 {
		t.Fatalf("expected non-zero code stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "state mismatch") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if _, ok := store.Accounts["main"]; ok {
		t.Fatalf("profile was saved on state mismatch: %+v", store)
	}
}

func TestAccountLoginInvalidSubmissionDoesNotStopLaterValidCode(t *testing.T) {
	path := testConfigPath(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse OAuth form: %v", err)
		}
		code := r.Form.Get("code")
		if code != "valid-after-invalid" {
			http.Error(w, "unexpected code", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "after-invalid-refresh-secret",
			"user":          map[string]any{"id": "97531"},
		})
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "account", "login", "--addr", addr, "--no-open", "--timeout", "5s", "main"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	waitForLoginServer(t, addr)
	resp, err := http.Get("http://" + addr + "/callback?code=wrong-state-direct&state=wrong-state")
	if err != nil {
		t.Fatalf("send wrong-state direct callback: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong-state direct callback status = %d", resp.StatusCode)
	}

	resp, err = http.Get("http://" + addr + "/callback?code=missing-state-direct")
	if err != nil {
		t.Fatalf("send invalid direct callback: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid direct callback status = %d", resp.StatusCode)
	}

	invalidCallback := "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback?code=missing-state-code"
	resp, err = http.PostForm("http://"+addr+"/manual", url.Values{"code": {invalidCallback}})
	if err != nil {
		t.Fatalf("post invalid callback URL: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid callback status = %d", resp.StatusCode)
	}

	resp, err = http.PostForm("http://"+addr+"/manual", url.Values{"code": {"pixiv://auth/callback?code=leaky-secret%zz"}})
	if err != nil {
		t.Fatalf("post malformed callback URL: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed callback status = %d", resp.StatusCode)
	}

	resp, err = http.PostForm("http://"+addr+"/manual", url.Values{"code": {"valid-after-invalid"}})
	if err != nil {
		t.Fatalf("post valid manual code: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if code := run.waitWithin(t, 5*time.Second); code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "state") {
		t.Fatalf("stderr did not include invalid submission reason: %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "leaky-secret") {
		t.Fatalf("stderr leaked malformed callback input: %q", stderr.String())
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if got := store.Accounts["main"].RefreshToken; got != "after-invalid-refresh-secret" {
		t.Fatalf("stored refresh token = %q", got)
	}
}

func TestAccountLoginUseSetsDefaultAndNoOpenSkipsBrowser(t *testing.T) {
	path := testConfigPath(t)
	if err := saveAccountStore(path, accountStore{
		DefaultProfile: "other",
		Accounts: map[string]account{
			"other": {RefreshToken: "other-refresh"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse OAuth form: %v", err)
		}
		if got := r.Form.Get("code"); got != "use-code" {
			t.Fatalf("code = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "use-refresh-secret",
			"user":          map[string]any{"id": "24680"},
		})
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()
	calledOpen := false
	restoreOpen := setTestOpenBrowser(t, func(string) error {
		calledOpen = true
		return errors.New("browser opener should not be called")
	})
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "account", "login", "--use", "--addr", addr, "--no-open", "--timeout", "5s", "main"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	waitForLoginServer(t, addr)
	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {"use-code"}})
	if err != nil {
		t.Fatalf("post manual code: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if code := run.waitWithin(t, 5*time.Second); code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if calledOpen {
		t.Fatalf("--no-open called browser opener")
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if store.DefaultProfile != "main" {
		t.Fatalf("default profile = %q", store.DefaultProfile)
	}
}

func TestAccountLoginOpenBrowserFailureWarnsAndManualCodeSucceeds(t *testing.T) {
	path := testConfigPath(t)
	addr := freeLoopbackAddr(t)
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse OAuth form: %v", err)
		}
		if got := r.Form.Get("code"); got != "after-open-fail" {
			t.Fatalf("code = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"refresh_token": "open-fail-refresh-secret",
			"user":          map[string]any{"id": "11223"},
		})
	}))
	defer oauth.Close()
	restoreOAuthBase := setTestOAuthBase(t, oauth.URL)
	defer restoreOAuthBase()
	restoreOpen := setTestOpenBrowser(t, func(string) error {
		return errors.New("opener unavailable")
	})
	defer restoreOpen()

	var stdout, stderr bytes.Buffer
	run := startAsyncCLIRun([]string{"pixiv", "account", "login", "--addr", addr, "--timeout", "5s", "main"}, strings.NewReader(""), &stdout, &stderr)
	defer run.wait()
	waitForLoginServer(t, addr)
	resp, err := http.PostForm("http://"+addr+"/manual", url.Values{"code": {"after-open-fail"}})
	if err != nil {
		t.Fatalf("post manual code: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if code := run.waitWithin(t, 5*time.Second); code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "warning") || !strings.Contains(stderr.String(), "opener unavailable") {
		t.Fatalf("stderr did not include opener warning: %q", stderr.String())
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if got := store.Accounts["main"].RefreshToken; got != "open-fail-refresh-secret" {
		t.Fatalf("stored refresh token = %q", got)
	}
}

func TestAccountAddReadsTokenFromStdin(t *testing.T) {
	path := testConfigPath(t)
	var stdout, stderr bytes.Buffer

	code := Run([]string{"pixiv", "account", "add", "main"}, strings.NewReader("stdin-token\n"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	store, err := loadAccountStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if store.Accounts["main"].RefreshToken != "stdin-token" {
		t.Fatalf("store = %+v", store)
	}
}

func TestAccountAddRejectsCookieWithoutRefreshToken(t *testing.T) {
	testConfigPath(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "account", "add", "--token", "PHPSESSID=web; device_token=device", "main"}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero code")
	}
	if !strings.Contains(stderr.String(), "refresh_token") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestClientConfigProfilePriority(t *testing.T) {
	path := testConfigPath(t)
	if err := saveAccountStore(path, accountStore{
		DefaultProfile: "main",
		Accounts: map[string]account{
			"main":  {RefreshToken: "main-token"},
			"other": {RefreshToken: "other-token"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXIV_REFRESH_TOKEN", "env-token")

	a := app{in: strings.NewReader(""), out: &bytes.Buffer{}, errOut: &bytes.Buffer{}}
	client, _, err := a.clientAndConfig(commandOptions{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if client.RefreshTokenValue() != "env-token" {
		t.Fatalf("token = %q", client.RefreshTokenValue())
	}
	client, _, err = a.clientAndConfig(commandOptions{profile: "other"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if client.RefreshTokenValue() != "other-token" {
		t.Fatalf("token = %q", client.RefreshTokenValue())
	}
	client, _, err = a.clientAndConfig(commandOptions{profile: "other", refreshToken: "flag-token"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if client.RefreshTokenValue() != "flag-token" {
		t.Fatalf("token = %q", client.RefreshTokenValue())
	}
}

func testConfigPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pixiv", "config.json")
	old := configPath
	configPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { configPath = old })
	return path
}

func setTestOAuthBase(t *testing.T, baseURL string) func() {
	t.Helper()
	return setLoginOAuthBaseForTest(baseURL)
}

func setTestOpenBrowser(t *testing.T, opener func(string) error) func() {
	t.Helper()
	return setOpenBrowserForTest(opener)
}

type asyncCLIRun struct {
	done     chan int
	mu       sync.Mutex
	received bool
	code     int
}

func startAsyncCLIRun(args []string, in io.Reader, out io.Writer, errOut io.Writer) *asyncCLIRun {
	run := &asyncCLIRun{done: make(chan int, 1)}
	go func() {
		run.done <- Run(args, in, out, errOut)
	}()
	return run
}

func (r *asyncCLIRun) wait() int {
	r.mu.Lock()
	if r.received {
		code := r.code
		r.mu.Unlock()
		return code
	}
	r.mu.Unlock()

	code := <-r.done
	r.mu.Lock()
	if !r.received {
		r.code = code
		r.received = true
	}
	code = r.code
	r.mu.Unlock()
	return code
}

func (r *asyncCLIRun) waitWithin(t *testing.T, timeout time.Duration) int {
	t.Helper()
	r.mu.Lock()
	if r.received {
		code := r.code
		r.mu.Unlock()
		return code
	}
	r.mu.Unlock()

	select {
	case code := <-r.done:
		r.mu.Lock()
		if !r.received {
			r.code = code
			r.received = true
		}
		code = r.code
		r.mu.Unlock()
		return code
	case <-time.After(timeout):
		t.Fatalf("login command did not finish")
		return 1
	}
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on loopback: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close loopback listener: %v", err)
	}
	return addr
}

func waitForLoginServer(t *testing.T, addr string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return string(body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("login server did not start at %s", addr)
	return ""
}

func loginURLFromPage(t *testing.T, page string) string {
	t.Helper()
	const marker = `href="`
	start := strings.Index(page, marker)
	if start < 0 {
		t.Fatalf("login page did not contain href: %s", page)
	}
	start += len(marker)
	end := strings.Index(page[start:], `"`)
	if end < 0 {
		t.Fatalf("login page href was not closed: %s", page)
	}
	return html.UnescapeString(page[start : start+end])
}

func loginState(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse login URL: %v", err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatalf("login URL did not include state: %s", rawURL)
	}
	return state
}
