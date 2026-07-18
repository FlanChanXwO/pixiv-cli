package pixiv_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestOpenDefaultClassifiesMalformedConfigLocalState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config-path-secret.toml")
	if err := os.WriteFile(configPath, []byte("config-body-secret = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := pixiv.OpenDefault(pixiv.Options{
		AuthFilePath:   filepath.Join(dir, "missing-auth.json"),
		ConfigFilePath: configPath,
	})
	if err != nil {
		t.Fatalf("OpenDefault() error = %v", err)
	}
	_, err = client.ListAccounts()
	assertSafeLocalStateError(t, err, pixiv.OperationListAccounts, pixiv.LocalStateKindConfigMalformed, "local configuration is malformed", "config-path-secret", "config-body-secret")
}

func TestOpenDefaultClassifiesMalformedAuthLocalState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth-path-secret.json")
	if err := os.WriteFile(authPath, []byte(`{"refresh_token":"auth-body-secret"`), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := pixiv.OpenDefault(pixiv.Options{
		AuthFilePath:   authPath,
		ConfigFilePath: filepath.Join(dir, "missing-config.toml"),
	})
	if err != nil {
		t.Fatalf("OpenDefault() error = %v", err)
	}
	_, err = client.ListAccounts()
	assertSafeLocalStateError(t, err, pixiv.OperationListAccounts, pixiv.LocalStateKindAuthMalformed, "local authentication is malformed", "auth-path-secret", "auth-body-secret")
}

func TestOpenDefaultClassifiesLegacyAuthSchemaLocalStateMalformed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "legacy-auth-path-secret.json")
	body := `{"default_account":"legacy-field-secret","accounts":[{"name":"legacy-name-secret","refresh_token":"legacy-token-secret"}]}`
	if err := os.WriteFile(authPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := pixiv.OpenDefault(pixiv.Options{
		AuthFilePath:   authPath,
		ConfigFilePath: filepath.Join(dir, "missing-config.toml"),
	})
	if err != nil {
		t.Fatalf("OpenDefault() error = %v", err)
	}
	_, err = client.ListAccounts()
	assertSafeLocalStateError(t, err, pixiv.OperationListAccounts, pixiv.LocalStateKindAuthMalformed, "local authentication is malformed", "legacy-auth-path-secret", "legacy-field-secret", "legacy-name-secret", "legacy-token-secret")
}

func TestOpenDefaultClassifiesInvalidProxyLocalState(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "proxy-config-path-secret.toml")
	proxy := "http://proxy-user-secret:proxy-pass-secret@proxy-host-secret.invalid/proxy-path-secret-%zz?proxy-query-secret=value"
	// lower-case 代理优先于文件与 upper-case；两者都固定为同一非法值，保证
	// 调用进程的环境不能覆盖本测试要验证的 proxy snapshot。
	t.Setenv("https_proxy", proxy)
	t.Setenv("HTTPS_PROXY", proxy)
	body := "[network]\nhttps_proxy = " + fmt.Sprintf("%q", proxy) + "\n"
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var networkCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		networkCalls.Add(1)
		http.Error(w, "unexpected oauth request", http.StatusInternalServerError)
	}))
	defer server.Close()
	var logs bytes.Buffer
	client, err := pixiv.OpenDefault(pixiv.Options{
		AuthFilePath:   filepath.Join(dir, "missing-auth.json"),
		ConfigFilePath: configPath,
		OAuthBaseURL:   server.URL,
		Logger:         slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatalf("OpenDefault() error = %v", err)
	}
	_, err = client.CheckRefreshToken(context.Background(), "unused-token")
	assertSafeLocalStateError(t, err, pixiv.OperationCheckRefreshToken, pixiv.LocalStateKindInvalidProxy, "configured proxy URL is invalid", "proxy-config-path-secret", "proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret", server.URL)
	if calls := networkCalls.Load(); calls != 0 {
		t.Fatalf("OAuth network calls = %d, want 0", calls)
	}
	if !strings.Contains(logs.String(), "pixiv operation") || !strings.Contains(logs.String(), "error_code=invalid_argument") {
		t.Fatalf("safe operation log missing classification: %q", logs.String())
	}
	for _, canary := range []string{"proxy-config-path-secret", "proxy-user-secret", "proxy-pass-secret", "proxy-host-secret", "proxy-path-secret", "proxy-query-secret", server.URL} {
		if strings.Contains(logs.String(), canary) {
			t.Fatalf("operation log leaked %q in %q", canary, logs.String())
		}
	}
}

func TestOpenDefaultMissingOptionalLocalStateRemainsEmptySuccess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	client, err := pixiv.OpenDefault(pixiv.Options{
		AuthFilePath:   filepath.Join(dir, "missing-auth.json"),
		ConfigFilePath: filepath.Join(dir, "missing-config.toml"),
	})
	if err != nil {
		t.Fatalf("OpenDefault() error = %v", err)
	}
	accounts, err := client.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if accounts.DefaultUserID != 0 || len(accounts.Accounts) != 0 {
		t.Fatalf("ListAccounts() = %#v, want empty state", accounts)
	}
}

func TestErrorNormalizesUnrecognizedLocalStateKind(t *testing.T) {
	t.Parallel()
	raw := "https://local-user:local-pass@host.invalid/local-path?token=local-token Cookie: local-cookie"
	typed := &pixiv.Error{
		Code:           pixiv.CodeInvalidArgument,
		Operation:      pixiv.OperationConfigGet,
		LocalStateKind: pixiv.LocalStateKind(raw),
	}
	for _, rendered := range []string{typed.Error(), fmt.Sprintf("%v", typed), fmt.Sprintf("%+v", typed)} {
		if strings.Contains(rendered, raw) || strings.Contains(rendered, "local-token") || strings.Contains(rendered, "local-cookie") {
			t.Fatalf("unrecognized local state kind leaked: %q", rendered)
		}
		if !strings.Contains(rendered, "local_state_kind=unknown") {
			t.Fatalf("rendered error = %q, want normalized unknown kind", rendered)
		}
	}
	if errors.Unwrap(typed) != nil {
		t.Fatalf("errors.Unwrap() = %v, want nil", errors.Unwrap(typed))
	}
}

func TestErrorPreservesDeclaredLocalStateKinds(t *testing.T) {
	t.Parallel()
	for _, kind := range []pixiv.LocalStateKind{
		pixiv.LocalStateKindAuthMalformed,
		pixiv.LocalStateKindConfigMalformed,
		pixiv.LocalStateKindPermissionDenied,
		pixiv.LocalStateKindNotFound,
		pixiv.LocalStateKindInvalidProxy,
		pixiv.LocalStateKindAccountMismatch,
		pixiv.LocalStateKindUnavailable,
		pixiv.LocalStateKindUnknown,
	} {
		diagnostic := (&pixiv.Error{Code: pixiv.CodeInvalidArgument, LocalStateKind: kind}).Error()
		if !strings.Contains(diagnostic, "local_state_kind="+string(kind)) {
			t.Errorf("kind %q diagnostic = %q", kind, diagnostic)
		}
	}
	if diagnostic := (&pixiv.Error{Code: pixiv.CodeInvalidArgument}).Error(); strings.Contains(diagnostic, "local_state_kind=") {
		t.Fatalf("empty kind diagnostic = %q", diagnostic)
	}
}

func TestErrorNormalizesLocalWriteCommitOutcome(t *testing.T) {
	t.Parallel()
	for _, outcome := range []pixiv.LocalWriteCommitOutcome{
		pixiv.LocalWriteCommitOutcomeUnknown,
		pixiv.LocalWriteCommitOutcomeNotCommitted,
		pixiv.LocalWriteCommitOutcomeCommitted,
	} {
		diagnostic := (&pixiv.Error{Code: pixiv.CodeInvalidArgument, LocalWriteCommitOutcome: outcome}).Error()
		if !strings.Contains(diagnostic, "local_write_commit_outcome="+string(outcome)) {
			t.Errorf("outcome %q diagnostic=%q", outcome, diagnostic)
		}
	}
	raw := pixiv.LocalWriteCommitOutcome("source-secret")
	diagnostic := (&pixiv.Error{Code: pixiv.CodeInvalidArgument, LocalWriteCommitOutcome: raw}).Error()
	if strings.Contains(diagnostic, string(raw)) || !strings.Contains(diagnostic, "local_write_commit_outcome=unknown") {
		t.Fatalf("diagnostic=%q", diagnostic)
	}
}

func TestNonLocalErrorLeavesLocalStateKindEmpty(t *testing.T) {
	t.Parallel()
	client, err := pixiv.NewClient(pixiv.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.IllustDetail(context.Background(), 0)
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *pixiv.Error", err)
	}
	if typed.LocalStateKind != "" {
		t.Fatalf("LocalStateKind = %q, want empty", typed.LocalStateKind)
	}
}

func TestCheckAccountClassifiesOAuthIdentityLocalStateMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "mismatch-auth-path-secret.json")
	if err := os.WriteFile(authPath, []byte(`{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":"mismatch-stored-token-secret"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" {
			t.Errorf("request path = %q", r.URL.Path)
			http.Error(w, "unexpected request path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"mismatch-access-token-secret","refresh_token":"mismatch-rotated-token-secret","user":{"id":8}}`))
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath, HTTPClient: server.Client(), OAuthBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CheckAccount(context.Background(), 7)
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationCheckAccount || typed.Backend != pixiv.BackendOAuth || typed.UserID != 7 || typed.Retryable || typed.LocalStateKind != pixiv.LocalStateKindAccountMismatch {
		t.Fatalf("mismatch error = %#v", err)
	}
	cause := errors.Unwrap(err)
	if cause == nil || cause.Error() != "oauth identity does not match selected account" {
		t.Fatalf("mismatch cause = %v", cause)
	}
	for _, rendered := range []string{err.Error(), cause.Error(), fmt.Sprintf("%+v", typed)} {
		for _, canary := range []string{"mismatch-auth-path-secret", "mismatch-stored-token-secret", "mismatch-access-token-secret", "mismatch-rotated-token-secret", server.URL} {
			if strings.Contains(rendered, canary) {
				t.Fatalf("mismatch canary %q leaked: %q", canary, rendered)
			}
		}
	}
}

func assertSafeLocalStateError(t *testing.T, err error, operation pixiv.Operation, kind pixiv.LocalStateKind, safeCause string, canaries ...string) {
	t.Helper()
	var typed *pixiv.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *pixiv.Error", err)
	}
	if typed.Code != pixiv.CodeInvalidArgument || typed.Operation != operation || typed.Backend != "" || typed.UserID != 0 || typed.Retryable || typed.LocalStateKind != kind {
		t.Fatalf("error = %#v", typed)
	}
	cause := errors.Unwrap(err)
	if cause == nil || cause.Error() != safeCause {
		t.Fatalf("cause = %v, want %q", cause, safeCause)
	}
	renderings := []string{err.Error(), cause.Error(), fmt.Sprintf("%+v", typed)}
	for _, rendered := range renderings {
		for _, canary := range canaries {
			if strings.Contains(rendered, canary) {
				t.Fatalf("sensitive canary %q leaked in %q", canary, rendered)
			}
		}
	}
}
