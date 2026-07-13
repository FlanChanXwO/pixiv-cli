package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
)

func TestExplicitAccountStoreRefreshesRotatedTokenWithoutExposingIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"default_user_id":7,"accounts":[{"user_id":7,"username":"old","refresh_token":"old-refresh-secret"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("refresh_token") != "old-refresh-secret" {
			t.Fatalf("form=%v err=%v", r.Form, err)
		}
		_, _ = w.Write([]byte(`{"access_token":"access-secret","refresh_token":"rotated-refresh-secret","user":{"id":7,"name":"new"}}`))
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath, OAuthBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	account, err := client.CheckAccount(context.Background(), 7)
	if err != nil || account.UserID != 7 || account.Username != "new" || !account.Default || !account.HasToken {
		t.Fatalf("account=%+v err=%v", account, err)
	}
	listed, err := client.ListAccounts()
	if err != nil || len(listed.Accounts) != 1 {
		t.Fatalf("list=%+v err=%v", listed, err)
	}
	wire, _ := json.Marshal(listed)
	for _, secret := range []string{"old-refresh-secret", "rotated-refresh-secret", "access-secret"} {
		if strings.Contains(string(wire), secret) || strings.Contains(errString(err), secret) {
			t.Fatalf("secret exposed: %s", secret)
		}
	}
	body, err := os.ReadFile(authPath)
	if err != nil || !strings.Contains(string(body), "rotated-refresh-secret") || strings.Contains(string(body), "old-refresh-secret") {
		t.Fatalf("auth store not rotated: %s err=%v", body, err)
	}
}

func TestAccountAndConfigRequireExplicitStoreOnNewClient(t *testing.T) {
	t.Parallel()
	client, err := pixiv.NewClient(pixiv.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListAccounts(); !errors.Is(err, pixiv.ErrUnsupported) {
		t.Fatalf("ListAccounts error=%v", err)
	}
	if _, err := client.GetConfig("web_fallback_enabled"); !errors.Is(err, pixiv.ErrUnsupported) {
		t.Fatalf("GetConfig error=%v", err)
	}
}

func TestConfigUsesExistingAliasesPrivateFileAndEnvPriority(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.toml")
	client, err := pixiv.NewClient(pixiv.Options{ConfigFilePath: path})
	if err != nil {
		t.Fatal(err)
	}
	value, err := client.SetConfig("web_fallback_enabled", "false")
	if err != nil || value.Source != "file" || value.Value != "false" {
		t.Fatalf("set=%+v err=%v", value, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode(), err)
	}
	got, err := client.GetConfig("web_fallback_enabled")
	if err != nil || got.Value != "false" || got.Source != "file" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	if removed, err := client.UnsetConfig("web_fallback_enabled"); err != nil || !removed {
		t.Fatalf("unset removed=%v err=%v", removed, err)
	}
	if _, err := client.SetConfig("unknown", "x"); !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("invalid key error=%v", err)
	}
}

func TestConfigGetPrefersExistingProxyEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[network]\nhttps_proxy = 'http://file.invalid:7890'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("https_proxy", "http://environment.invalid:7890")
	client, err := pixiv.NewClient(pixiv.Options{ConfigFilePath: path})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetConfig("https_proxy")
	if err != nil || got.Source != "env" || got.Value != "http://environment.invalid:7890" {
		t.Fatalf("config=%+v err=%v", got, err)
	}
}

func TestLoginSessionIsOpaqueOneTimeAndValidatesCallback(t *testing.T) {
	t.Parallel()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	var exchanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges.Add(1)
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "code-secret" || r.Form.Get("code_verifier") == "" {
			t.Fatalf("form=%v err=%v", r.Form, err)
		}
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh-secret","user":{"id":9,"name":"alice"}}`))
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath, OAuthBaseURL: server.URL, AppAPIBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.StartLogin()
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(session)
	if string(encoded) != "{}" {
		t.Fatalf("session JSON=%s", encoded)
	}
	for _, rendered := range []string{fmt.Sprint(session), fmt.Sprintf("%+v", session), fmt.Sprintf("%#v", session), fmt.Sprintf("%q", session)} {
		if strings.Contains(rendered, "code_challenge") || strings.Contains(rendered, "state=") || strings.Contains(rendered, "pixiv-android") {
			t.Fatalf("login session formatting leaked: %q", rendered)
		}
	}
	loginURL, err := url.Parse(session.AuthorizationURL())
	if err != nil || loginURL.Path != "/web/v1/login" || loginURL.Query().Get("code_challenge_method") != "S256" || loginURL.Query().Get("state") == "" {
		t.Fatalf("authorization url=%q err=%v", session.AuthorizationURL(), err)
	}
	if _, err := client.CompleteLogin(context.Background(), session, "https://example.invalid/callback?code=bad&state=wrong", pixiv.LoginOptions{}); !errors.Is(err, pixiv.ErrInvalidArgument) || exchanges.Load() != 0 {
		t.Fatalf("wrong callback err=%v exchanges=%d", err, exchanges.Load())
	}
	session, _ = client.StartLogin()
	state := mustQuery(t, session.AuthorizationURL(), "state")
	account, err := client.CompleteLogin(context.Background(), session, "https://example.invalid/callback?code=code-secret&state="+url.QueryEscape(state), pixiv.LoginOptions{UseAsDefault: true})
	if err != nil || account.UserID != 9 || !account.Default {
		t.Fatalf("complete account=%+v err=%v", account, err)
	}
	if _, err := client.CompleteLogin(context.Background(), session, "code-secret", pixiv.LoginOptions{}); !errors.Is(err, pixiv.ErrInvalidArgument) || exchanges.Load() != 1 {
		t.Fatalf("reuse err=%v exchanges=%d", err, exchanges.Load())
	}
	other, _ := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath})
	if _, err := other.CompleteLogin(context.Background(), session, "code-secret", pixiv.LoginOptions{}); !errors.Is(err, pixiv.ErrInvalidArgument) || exchanges.Load() != 1 {
		t.Fatalf("cross client err=%v exchanges=%d", err, exchanges.Load())
	}
}

func TestLoginSessionConcurrentCompletionExchangesOnce(t *testing.T) {
	t.Parallel()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	var exchanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges.Add(1)
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","user":{"id":10}}`))
	}))
	defer server.Close()
	client, _ := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath, OAuthBaseURL: server.URL, HTTPClient: server.Client()})
	session, _ := client.StartLogin()
	copyValue := *session
	var wg sync.WaitGroup
	for _, current := range []*pixiv.LoginSession{session, &copyValue} {
		wg.Add(1)
		go func(current *pixiv.LoginSession) {
			defer wg.Done()
			_, _ = client.CompleteLogin(context.Background(), current, "code", pixiv.LoginOptions{})
		}(current)
	}
	wg.Wait()
	if exchanges.Load() != 1 {
		t.Fatalf("exchanges=%d", exchanges.Load())
	}
}

func TestOpenDefaultReadsOneCurrentSnapshotPerOperation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[web]\nfallback_enabled = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var webCalls, appCalls, oauthCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			oauthCalls.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"current-access","refresh_token":"rotated","user":{"id":7}}`))
		case "/ajax/illust/1/pages":
			webCalls.Add(1)
			if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
				t.Fatalf("web carried SDK auth")
			}
			_, _ = w.Write([]byte(`{"error":false,"body":[{"urls":{"original":"https://i.pximg.net/img-original/x.jpg"},"width":1,"height":1}]}`))
		case "/v1/illust/detail":
			appCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer current-access" {
				t.Fatalf("app Authorization=%q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"illust":{"id":1,"type":"illust","page_count":1,"user":{},"tags":[],"image_urls":{},"meta_single_page":{},"meta_pages":[]}}`))
		default:
			t.Fatalf("unexpected path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := pixiv.OpenDefault(pixiv.Options{AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: server.Client(), OAuthBaseURL: server.URL, AppAPIBaseURL: server.URL, WebAPIBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.IllustPages(context.Background(), 1); err != nil || webCalls.Load() != 1 || oauthCalls.Load() != 0 {
		t.Fatalf("anonymous pages err=%v web=%d oauth=%d", err, webCalls.Load(), oauthCalls.Load())
	}
	if err := os.WriteFile(authPath, []byte(`{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":"stored"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.IllustDetail(context.Background(), 1); err != nil || appCalls.Load() != 1 || webCalls.Load() != 2 || oauthCalls.Load() != 1 {
		t.Fatalf("authenticated detail err=%v app=%d web=%d oauth=%d", err, appCalls.Load(), webCalls.Load(), oauthCalls.Load())
	}
	if body, err := os.ReadFile(authPath); err != nil || !strings.Contains(string(body), "rotated") || strings.Contains(string(body), `"refresh_token":"stored"`) {
		t.Fatalf("snapshot rotation was not saved: body=%s err=%v", body, err)
	}
	if err := os.Remove(authPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("[web]\nfallback_enabled = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.IllustDetail(context.Background(), 1); !errors.Is(err, pixiv.ErrUnauthorized) || appCalls.Load() != 1 || webCalls.Load() != 2 {
		t.Fatalf("changed snapshot error=%v app=%d web=%d", err, appCalls.Load(), webCalls.Load())
	}
}

func TestOpenDefaultCurrentUserIDUsesOAuthSnapshotIdentity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(authPath, []byte(`{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":"stored"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"rotated","user":{"id":7}}`))
	}))
	defer server.Close()
	client, err := pixiv.OpenDefault(pixiv.Options{AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: server.Client(), OAuthBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := client.CurrentUserID(context.Background())
	if err != nil || userID != 7 {
		t.Fatalf("CurrentUserID()=(%d,%v)", userID, err)
	}
}

func TestOpenDefaultCurrentUserIDPrefersExplicitAndEnvironmentOAuthIdentity(t *testing.T) {
	// t.Setenv 不能与 t.Parallel 共用；此回归刻意验证 token 优先级与本地 default UID
	// 不一致时，CLI 所需的“当前用户”仍来自本次 OAuth 响应。
	for _, test := range []struct {
		name          string
		explicitToken string
		envToken      string
		oauthUserID   int64
	}{
		{name: "explicit token", explicitToken: "explicit-token", oauthUserID: 101},
		{name: "environment token", envToken: "environment-token", oauthUserID: 202},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			authPath := filepath.Join(dir, "auth.json")
			configPath := filepath.Join(dir, "config.toml")
			if err := os.WriteFile(authPath, []byte(`{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":"stored-token"}]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if test.envToken != "" {
				t.Setenv("PIXIV_REFRESH_TOKEN", test.envToken)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/auth/token" {
					t.Fatalf("path=%s", r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				expectedToken := test.explicitToken
				if expectedToken == "" {
					expectedToken = test.envToken
				}
				if got := r.Form.Get("refresh_token"); got != expectedToken {
					t.Fatalf("refresh_token=%q want=%q", got, expectedToken)
				}
				_, _ = fmt.Fprintf(w, `{"access_token":"access","refresh_token":"rotated","user":{"id":%d}}`, test.oauthUserID)
			}))
			defer server.Close()
			client, err := pixiv.OpenDefault(pixiv.Options{AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: server.Client(), OAuthBaseURL: server.URL, RefreshToken: test.explicitToken})
			if err != nil {
				t.Fatal(err)
			}
			userID, err := client.CurrentUserID(context.Background())
			if err != nil || userID != test.oauthUserID {
				t.Fatalf("CurrentUserID()=(%d,%v), want %d", userID, err, test.oauthUserID)
			}
		})
	}
}

func TestSnapshotKeepsConfigUntilNextExplicitSnapshot(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[web]\nfallback_enabled = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ajax/illust/1/pages" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"error":false,"body":[{"urls":{"original":"https://i.pximg.net/img-original/x.jpg"},"width":1,"height":1}]}`))
	}))
	defer server.Close()
	client, err := pixiv.OpenDefault(pixiv.Options{AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: server.Client(), WebAPIBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("[web]\nfallback_enabled = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := first.IllustPages(context.Background(), 1); err != nil {
		t.Fatalf("first snapshot pages: %v", err)
	}
	second, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.IllustPages(context.Background(), 1); !errors.Is(err, pixiv.ErrUnauthorized) {
		t.Fatalf("second snapshot pages error=%v", err)
	}
}

func TestOpenDefaultCursorRejectsSourceChangeAndLegacyCursor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[web]\nfallback_enabled = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeAuth := func(token string, userID int) {
		t.Helper()
		if err := os.WriteFile(authPath, []byte(`{"default_user_id":`+strconv.Itoa(userID)+`,"accounts":[{"user_id":`+strconv.Itoa(userID)+`,"refresh_token":"`+token+`"}]}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeAuth("uid-seven", 7)
	var contentCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			_ = r.ParseForm()
			uid := 7
			if r.Form.Get("refresh_token") == "uid-eight" {
				uid = 8
			}
			_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"rotated","user":{"id":` + strconv.Itoa(uid) + `}}`))
		case "/v1/search/illust":
			contentCalls.Add(1)
			if r.URL.Query().Get("offset") == "" {
				_, _ = w.Write([]byte(`{"illusts":[],"next_url":"/v1/search/illust?offset=30&token=never-in-cursor"}`))
				return
			}
			_, _ = w.Write([]byte(`{"illusts":[],"next_url":null}`))
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	options := pixiv.Options{AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: server.Client(), OAuthBaseURL: server.URL, AppAPIBaseURL: server.URL}
	client, err := pixiv.OpenDefault(options)
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku"})
	if err != nil || first.NextCursor == "" || strings.Contains(string(first.NextCursor), "never-in-cursor") {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	writeAuth("uid-eight", 8)
	if _, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: first.NextCursor}); !errors.Is(err, pixiv.ErrInvalidArgument) || contentCalls.Load() != 1 {
		t.Fatalf("source change err=%v content=%d", err, contentCalls.Load())
	}
	writeAuth("uid-seven", 7)
	if err := os.WriteFile(configPath, []byte("[web]\nfallback_enabled = true\n[output]\njson = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: first.NextCursor}); err != nil || contentCalls.Load() != 2 {
		t.Fatalf("same source continuation err=%v content=%d", err, contentCalls.Load())
	}
	// Direct cursors remain compatible with their direct NewClient, but OpenDefault
	// rejects them because they carry no snapshot source identity.
	direct, _ := pixiv.NewClient(pixiv.Options{HTTPClient: server.Client(), AppAPIBaseURL: server.URL, AccessToken: "direct"})
	legacy, err := direct.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku"})
	if err != nil || legacy.NextCursor == "" {
		t.Fatalf("legacy=%+v err=%v", legacy, err)
	}
	if _, err := client.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Cursor: legacy.NextCursor}); !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("source-less cursor error=%v", err)
	}
}

func TestAccountLegacyAndOAuthFailureStaySafe(t *testing.T) {
	t.Parallel()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"default_account":"old","accounts":[{"name":"old","refresh_token":"legacy-secret"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	client, _ := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath})
	if _, err := client.ListAccounts(); !errors.Is(err, pixiv.ErrInvalidArgument) {
		t.Fatalf("legacy error=%v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "oauth-body-secret", http.StatusBadGateway)
	}))
	defer server.Close()
	if err := os.WriteFile(authPath, []byte(`{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":"stored-secret"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	client, _ = pixiv.NewClient(pixiv.Options{AuthFilePath: authPath, OAuthBaseURL: server.URL, HTTPClient: server.Client()})
	_, err := client.CheckAccount(context.Background(), 7)
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Backend != pixiv.BackendOAuth || typed.UpstreamStatus != http.StatusBadGateway {
		t.Fatalf("oauth error=%#v", err)
	}
	for _, rendered := range []string{err.Error(), errors.Unwrap(err).Error()} {
		if strings.Contains(rendered, "oauth-body-secret") || strings.Contains(rendered, "stored-secret") {
			t.Fatalf("secret leaked: %q", rendered)
		}
	}
}

func TestOpenDefaultRefreshTokenPriority(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(authPath, []byte(`{"default_user_id":1,"accounts":[{"user_id":1,"refresh_token":"default-token"},{"user_id":2,"refresh_token":"selected-token"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotToken atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			token := r.Form.Get("refresh_token")
			gotToken.Store(token)
			userID := 1
			if token == "selected-token" {
				userID = 2
			}
			_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"rotated","user":{"id":` + strconv.Itoa(userID) + `}}`))
		case "/v1/illust/recommended":
			_, _ = w.Write([]byte(`{"illusts":[],"next_url":null}`))
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("PIXIV_REFRESH_TOKEN", "environment-token")
	call := func(options pixiv.Options, want string) {
		t.Helper()
		options.AuthFilePath, options.ConfigFilePath = authPath, configPath
		options.HTTPClient, options.OAuthBaseURL, options.AppAPIBaseURL = server.Client(), server.URL, server.URL
		client, err := pixiv.OpenDefault(options)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{}); err != nil {
			t.Fatal(err)
		}
		if got := gotToken.Load(); got != want {
			t.Fatalf("refresh token=%q want=%q", got, want)
		}
	}
	call(pixiv.Options{RefreshToken: "explicit-token", UserID: 2}, "explicit-token")
	call(pixiv.Options{UserID: 2}, "selected-token")
	call(pixiv.Options{}, "environment-token")
	t.Setenv("PIXIV_REFRESH_TOKEN", "")
	call(pixiv.Options{}, "default-token")
}

func TestOpenDefaultMissingSelectedUIDAndStoredMismatchNeverReachContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(authPath, []byte(`{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":"stored-token"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var oauthCalls, contentCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			oauthCalls.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"wrong-rotated","user":{"id":8}}`))
		case "/v1/illust/recommended":
			contentCalls.Add(1)
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	options := pixiv.Options{AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: server.Client(), OAuthBaseURL: server.URL, AppAPIBaseURL: server.URL}
	missing, _ := pixiv.OpenDefault(pixiv.Options{AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: server.Client(), OAuthBaseURL: server.URL, AppAPIBaseURL: server.URL, UserID: 99})
	if _, err := missing.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{}); !errors.Is(err, pixiv.ErrInvalidArgument) || oauthCalls.Load() != 0 || contentCalls.Load() != 0 {
		t.Fatalf("missing selected uid err=%v oauth=%d content=%d", err, oauthCalls.Load(), contentCalls.Load())
	}
	client, _ := pixiv.OpenDefault(options)
	_, err := client.IllustRecommended(context.Background(), pixiv.IllustRecommendedRequest{})
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.UserID != 7 || typed.Operation != pixiv.OperationIllustRecommended || contentCalls.Load() != 0 {
		t.Fatalf("mismatch err=%#v content=%d", err, contentCalls.Load())
	}
	body, readErr := os.ReadFile(authPath)
	if readErr != nil || strings.Contains(string(body), "wrong-rotated") || !strings.Contains(string(body), "stored-token") {
		t.Fatalf("stored account changed body=%s err=%v", body, readErr)
	}
}

func TestOpenDefaultParseResourceRefUsesLocalSnapshotWithoutOAuth(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[web]\nfallback_enabled = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	client, err := pixiv.OpenDefault(pixiv.Options{AuthFilePath: authPath, ConfigFilePath: configPath, HTTPClient: &http.Client{Transport: accountRoundTripperFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("network should not run")
	})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ParseResourceRef("https://i.pximg.net/img-original/a.jpg"); err != nil || requests.Load() != 0 {
		t.Fatalf("parse err=%v requests=%d", err, requests.Load())
	}
	if err := os.WriteFile(configPath, []byte("[web\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ParseResourceRef("https://i.pximg.net/img-original/a.jpg"); !errors.Is(err, pixiv.ErrInvalidArgument) || requests.Load() != 0 {
		t.Fatalf("updated snapshot err=%v requests=%d", err, requests.Load())
	}
}

func TestOpenDefaultFreezesResourcePolicy(t *testing.T) {
	t.Parallel()
	policy := pixiv.ResourcePolicy{Mirrors: []pixiv.ResourceMirrorPolicy{{Host: "mirror.invalid", PathPrefixes: []string{"/safe"}}}}
	client, err := pixiv.OpenDefault(pixiv.Options{ResourcePolicy: policy, AuthFilePath: filepath.Join(t.TempDir(), "auth.json"), ConfigFilePath: filepath.Join(t.TempDir(), "config.toml")})
	if err != nil {
		t.Fatal(err)
	}
	policy.Mirrors[0].Host = "evil.invalid"
	policy.Mirrors[0].PathPrefixes[0] = "/"
	if _, err := client.ParseResourceRef("https://evil.invalid/anything"); !errors.Is(err, pixiv.ErrForbidden) {
		t.Fatalf("mutated caller policy changed client: %v", err)
	}
}

func TestOpenDefaultResourcesDoNotRefreshOAuth(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath, configPath := filepath.Join(dir, "auth.json"), filepath.Join(dir, "config.toml")
	if err := os.WriteFile(authPath, []byte(`{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":"expired-token"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var oauthCalls, resourceCalls atomic.Int32
	transport := accountRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "oauth.invalid" {
			oauthCalls.Add(1)
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("expired")), Header: make(http.Header), Request: request}, nil
		}
		if request.URL.Host == "i.pximg.net" {
			resourceCalls.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("image")), Header: make(http.Header), Request: request}, nil
		}
		return nil, errors.New("unexpected host")
	})
	client, err := pixiv.OpenDefault(pixiv.Options{AuthFilePath: authPath, ConfigFilePath: configPath, OAuthBaseURL: "https://oauth.invalid", HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := client.ParseResourceRef("https://i.pximg.net/img-original/a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.OpenResource(context.Background(), pixiv.OpenResourceRequest{Ref: ref})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	destination := filepath.Join(dir, "image.jpg")
	if err := client.Download(context.Background(), ref, destination); err != nil {
		t.Fatal(err)
	}
	if oauthCalls.Load() != 0 || resourceCalls.Load() != 2 {
		t.Fatalf("oauth=%d resource=%d", oauthCalls.Load(), resourceCalls.Load())
	}
}

func TestAccountMutationsShareOneClientTransaction(t *testing.T) {
	t.Parallel()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		userID := 1
		if request.Form.Get("refresh_token") == "token-2" {
			userID = 2
		}
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"rotated-` + strconv.Itoa(userID) + `","user":{"id":` + strconv.Itoa(userID) + `}}`))
	}))
	defer server.Close()
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath, OAuthBaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, token := range []string{"token-1", "token-2"} {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			if _, err := client.ImportAccount(context.Background(), token); err != nil {
				t.Errorf("ImportAccount(%q): %v", token, err)
			}
		}(token)
	}
	wg.Wait()
	accounts, err := client.ListAccounts()
	if err != nil || len(accounts.Accounts) != 2 {
		t.Fatalf("imports list=%+v err=%v", accounts, err)
	}
	wg.Add(2)
	go func() { defer wg.Done(); _ = client.SelectAccount(2) }()
	go func() { defer wg.Done(); _ = client.RemoveAccount(1) }()
	wg.Wait()
	accounts, err = client.ListAccounts()
	if err != nil || len(accounts.Accounts) != 1 || accounts.Accounts[0].UserID != 2 || accounts.DefaultUserID != 2 {
		t.Fatalf("linearized mutations list=%+v err=%v", accounts, err)
	}
}

func mustQuery(t *testing.T, raw, key string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Query().Get(key)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type accountRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f accountRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
