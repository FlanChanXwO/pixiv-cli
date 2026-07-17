package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	storageauth "github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestAuthenticatedCanaryChildEnvReplacesHostProxyOverrides(t *testing.T) {
	base := []string{
		"PATH=/bin",
		"https_proxy=http://hostile-lower.invalid",
		"HTTPS_PROXY=http://hostile-upper.invalid",
		"PIXIV_REFRESH_TOKEN=hostile-token",
	}
	const proxy = "socks5h://127.0.0.1:7890"

	local := authenticatedCanaryChildEnvFrom(base, authenticatedCanaryAuth{kind: canaryAuthLocalStore}, proxy)
	assertCanaryEnvValue(t, local, "PATH", "/bin", 1)
	assertCanaryEnvValue(t, local, "https_proxy", proxy, 1)
	assertCanaryEnvValue(t, local, "HTTPS_PROXY", proxy, 1)
	assertCanaryEnvValue(t, local, "PIXIV_REFRESH_TOKEN", "", 0)

	explicit := authenticatedCanaryChildEnvFrom(base, authenticatedCanaryAuth{
		kind:         canaryAuthExplicitToken,
		refreshToken: "explicit-token",
	}, proxy)
	assertCanaryEnvValue(t, explicit, "https_proxy", proxy, 1)
	assertCanaryEnvValue(t, explicit, "HTTPS_PROXY", proxy, 1)
	assertCanaryEnvValue(t, explicit, "PIXIV_REFRESH_TOKEN", "explicit-token", 1)
}

func authenticatedCanaryChildEnvFrom(environ []string, auth authenticatedCanaryAuth, proxy string) []string {
	filtered := make([]string, 0, len(environ)+3)
	for _, entry := range environ {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			filtered = append(filtered, entry)
			continue
		}
		if name == "https_proxy" || name == "HTTPS_PROXY" || strings.EqualFold(name, "PIXIV_REFRESH_TOKEN") {
			continue
		}
		filtered = append(filtered, entry)
	}
	if auth.kind == canaryAuthExplicitToken {
		filtered = append(filtered, "PIXIV_REFRESH_TOKEN="+auth.refreshToken)
	}
	if proxy != "" {
		filtered = append(filtered, "https_proxy="+proxy, "HTTPS_PROXY="+proxy)
	}
	return filtered
}

func TestAuthenticatedSDKCanaryRejectsExternalBinary(t *testing.T) {
	t.Parallel()

	if err := validateAuthenticatedSDKCanaryBinary("/release/pixiv"); err == nil {
		t.Fatal("SDK search canary accepted PIXIV_E2E_BINARY")
	}
	if err := validateAuthenticatedSDKCanaryBinary(""); err != nil {
		t.Fatalf("SDK search canary rejected current-source execution: %v", err)
	}
}

func validateAuthenticatedSDKCanaryBinary(externalBinary string) error {
	if externalBinary != "" {
		return errors.New("PIXIV_E2E_BINARY is not supported by the authenticated SDK search canary; unset it to test the current source")
	}
	return nil
}

func TestAuthenticatedCanaryExplicitSnapshotKeepsAllStoresIsolated(t *testing.T) {
	defaultDir := t.TempDir()
	defaultAuthPath := filepath.Join(defaultDir, "auth.json")
	defaultBody := []byte(`{"default_user_id":9,"accounts":[{"user_id":9,"refresh_token":"default-store-token"}]}`)
	if err := os.WriteFile(defaultAuthPath, defaultBody, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(storageauth.SetAuthFilePathForTest(defaultAuthPath))
	t.Setenv("PIXIV_REFRESH_TOKEN", "hostile-environment-token")

	var oauthCalls atomic.Int32
	failures := &handlerFailures{}
	defer failures.requireEmpty(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/token" {
			failures.add("unexpected request path %q", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		oauthCalls.Add(1)
		if err := r.ParseForm(); err != nil {
			failures.add("parse OAuth form: %v", err)
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		if got := r.Form.Get("refresh_token"); got != "explicit-test-token" {
			failures.add("OAuth selected an unexpected refresh token")
			http.Error(w, "unexpected token", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"rotated","user":{"id":7}}`))
	}))
	defer server.Close()

	options, err := authenticatedCanarySDKOptions(t, authenticatedCanaryAuth{
		kind:         canaryAuthExplicitToken,
		refreshToken: "explicit-test-token",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	options.HTTPClient = server.Client()
	options.OAuthBaseURL = server.URL
	if _, err := openAuthenticatedCanarySnapshot(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	if got := oauthCalls.Load(); got != 1 {
		t.Fatalf("OAuth refresh calls = %d, want 1", got)
	}
	for _, path := range []string{options.AuthFilePath, options.ConfigFilePath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("isolated explicit-token path unexpectedly exists: %v", err)
		}
	}
	gotDefault, err := os.ReadFile(defaultAuthPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotDefault, defaultBody) {
		t.Fatal("explicit SDK snapshot modified the default auth store")
	}
}

func TestAuthenticatedCanaryLocalSnapshotIgnoresEnvironmentTokenAndPersistsRotation(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":"stored-token"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(storageauth.SetAuthFilePathForTest(authPath))
	t.Setenv("PIXIV_REFRESH_TOKEN", "hostile-environment-token")

	var oauthCalls atomic.Int32
	failures := &handlerFailures{}
	defer failures.requireEmpty(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			oauthCalls.Add(1)
			if err := r.ParseForm(); err != nil {
				failures.add("parse OAuth form: %v", err)
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			if r.Form.Get("refresh_token") != "stored-token" {
				failures.add("local OAuth did not select the stored account")
				http.Error(w, "unexpected token", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"rotated-token","user":{"id":7}}`))
		case "/v1/search/options":
			_, _ = w.Write([]byte(`{"illust":{"tool":{"options":[]}}}`))
		default:
			failures.add("unexpected request path %q", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	options, err := authenticatedCanarySDKOptions(t, authenticatedCanaryAuth{kind: canaryAuthLocalStore}, "")
	if err != nil {
		t.Fatal(err)
	}
	options.HTTPClient = server.Client()
	options.OAuthBaseURL = server.URL
	options.AppAPIBaseURL = server.URL
	snapshot, err := openAuthenticatedCanarySnapshot(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.SearchIllustOptions(context.Background(), pixiv.SearchIllustOptionsRequest{Word: "miku"}); err != nil {
		t.Fatal(err)
	}
	if got := oauthCalls.Load(); got != 1 {
		t.Fatalf("OAuth refresh calls = %d, want 1", got)
	}
	body, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("rotated-token")) || bytes.Contains(body, []byte("hostile-environment-token")) {
		t.Fatal("local SDK snapshot did not safely persist the selected account rotation")
	}
}

func TestAuthenticatedCanarySearchSnapshotRefreshesOAuthOnce(t *testing.T) {
	var oauthCalls atomic.Int32
	failures := &handlerFailures{}
	defer failures.requireEmpty(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			oauthCalls.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"access","refresh_token":"rotated","user":{"id":7}}`))
		case "/v1/search/options":
			_, _ = w.Write([]byte(`{"illust":{"tool":{"options":[]}}}`))
		case "/v1/search/illust":
			_, _ = w.Write([]byte(`{"illusts":[],"next_url":null}`))
		default:
			failures.add("unexpected request path %q", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	options := pixiv.Options{
		RefreshToken:   "explicit-test-token",
		AuthFilePath:   filepath.Join(t.TempDir(), "auth.json"),
		ConfigFilePath: filepath.Join(t.TempDir(), "config.toml"),
		HTTPClient:     server.Client(),
		OAuthBaseURL:   server.URL,
		AppAPIBaseURL:  server.URL,
	}
	snapshot, err := openAuthenticatedCanarySnapshot(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.SearchIllustOptions(context.Background(), pixiv.SearchIllustOptionsRequest{Word: "miku"}); err != nil {
		t.Fatal(err)
	}
	for _, filters := range []pixiv.SearchIllustFilters{
		{},
		{Resolution: pixiv.SearchResolutionHigh},
		{AspectRatio: pixiv.SearchAspectRatioLandscape},
		{ContentType: pixiv.SearchContentTypeIllust},
		{AIMode: pixiv.SearchAIModeExclude},
		{Tool: "tool"},
	} {
		if _, err := snapshot.SearchIllust(context.Background(), pixiv.SearchIllustRequest{Word: "miku", Filters: filters}); err != nil {
			t.Fatal(err)
		}
	}
	if got := oauthCalls.Load(); got != 1 {
		t.Fatalf("OAuth refresh calls = %d, want 1 for the complete search canary", got)
	}
}

func authenticatedCanaryHTTPClient(proxyValue string) (*http.Client, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default HTTP transport has unexpected type %T", http.DefaultTransport)
	}
	transport := base.Clone()
	// 真实 canary 只服从显式代理，不能继承 DefaultTransport 的环境代理；client
	// 不设固定 timeout，请求生命周期继续由测试 context 或 go test 控制。
	transport.Proxy = nil
	if proxyValue != "" {
		parsed, err := url.ParseRequestURI(proxyValue)
		if err != nil || parsed.Host == "" {
			return nil, errors.New("PIXIV_E2E_PROXY must be an absolute http, https, socks5, or socks5h URL")
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, errors.New("PIXIV_E2E_PROXY must use http, https, socks5, or socks5h")
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	return &http.Client{Transport: transport}, nil
}

func TestAuthenticatedCanaryHTTPClientIsolatesEnvironmentAndRedactsProxyErrors(t *testing.T) {
	t.Setenv("https_proxy", "http://hostile-lower.invalid")
	t.Setenv("HTTPS_PROXY", "http://hostile-upper.invalid")
	client, err := authenticatedCanaryHTTPClient("")
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("isolated HTTP client transport=%T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil || client.Timeout != 0 {
		t.Fatalf("isolated HTTP client has_proxy=%t timeout=%v", transport.Proxy != nil, client.Timeout)
	}

	const secret = "proxy-password-secret"
	if _, err := authenticatedCanaryHTTPClient("https://user:" + secret + "@"); err == nil {
		t.Fatal("malformed authenticated canary proxy unexpectedly succeeded")
	} else if strings.Contains(err.Error(), secret) {
		t.Fatalf("proxy parse error leaked credentials: %v", err)
	}
}

func authenticatedCanarySDKOptions(t *testing.T, auth authenticatedCanaryAuth, proxy string) (pixiv.Options, error) {
	t.Helper()

	dir := t.TempDir()
	httpClient, err := authenticatedCanaryHTTPClient(proxy)
	if err != nil {
		return pixiv.Options{}, err
	}
	options := pixiv.Options{ConfigFilePath: filepath.Join(dir, "config.toml"), HTTPClient: httpClient}
	switch auth.kind {
	case canaryAuthExplicitToken:
		options.AuthFilePath = filepath.Join(dir, "auth.json")
		options.RefreshToken = auth.refreshToken
		return options, nil
	case canaryAuthLocalStore:
		client, err := pixiv.OpenDefault(options)
		if err != nil {
			return pixiv.Options{}, err
		}
		accounts, err := client.ListAccounts()
		if err != nil {
			return pixiv.Options{}, err
		}
		for _, account := range accounts.Accounts {
			if account.UserID == accounts.DefaultUserID && account.Default && account.HasToken {
				options.UserID = account.UserID
				return options, nil
			}
		}
		return pixiv.Options{}, errors.New("local authenticated canary requires a default account with a refresh token")
	default:
		return pixiv.Options{}, errors.New("authenticated canary credential mode is required")
	}
}

func openAuthenticatedCanarySnapshot(ctx context.Context, options pixiv.Options) (*pixiv.Client, error) {
	client, err := pixiv.OpenDefault(options)
	if err != nil {
		return nil, err
	}
	return client.Snapshot(ctx)
}

// TestPixivSDKAuthenticatedAppAPICanarySearchFilters 只验收当前源码的 public
// SDK 搜索能力。整个测试固定一个认证 snapshot，因此 exact -run 只刷新一次 OAuth。
func TestPixivSDKAuthenticatedAppAPICanarySearchFilters(t *testing.T) {
	if os.Getenv("PIXIV_E2E_REAL_API") != "1" {
		t.Skip("set PIXIV_E2E_REAL_API=1 to run authenticated App API search canary")
	}
	auth, skip, err := resolveAuthenticatedCanaryAuth(os.Getenv("PIXIV_E2E_REFRESH_TOKEN"), os.Getenv("PIXIV_E2E_USE_LOCAL_AUTH"))
	if err != nil {
		t.Fatalf("invalid authenticated App API canary credential mode: %v", err)
	}
	if skip {
		t.Skip("set PIXIV_E2E_REFRESH_TOKEN or PIXIV_E2E_USE_LOCAL_AUTH=1 with PIXIV_E2E_REAL_API=1 to run authenticated App API search canary")
	}
	if err := validateAuthenticatedSDKCanaryBinary(os.Getenv("PIXIV_E2E_BINARY")); err != nil {
		t.Fatal(err)
	}
	proxy := firstNonEmpty(os.Getenv("PIXIV_E2E_PROXY"), os.Getenv("PIXIV_WEB_API_PROXY"))
	options, err := authenticatedCanarySDKOptions(t, auth, proxy)
	if err != nil {
		t.Fatalf("prepare authenticated search canary SDK: %v", err)
	}
	client, err := openAuthenticatedCanarySnapshot(testCommandContext(t), options)
	if err != nil {
		t.Fatalf("open authenticated search canary snapshot: %v", err)
	}
	runAuthenticatedSearchCanary(t, client)
}

func runAuthenticatedSearchCanary(t *testing.T, client *pixiv.Client) {
	t.Helper()

	const searchWord = "初音ミク"
	baseline, err := searchCanaryNonempty(testCommandContext(t), client, pixiv.SearchIllustRequest{Word: searchWord})
	if err != nil {
		t.Fatalf("authenticated App search baseline failed: %v", err)
	}
	if len(baseline.Illusts) == 0 {
		t.Fatal("authenticated App search baseline returned no illustrations")
	}
	searchOptions, err := client.SearchIllustOptions(testCommandContext(t), pixiv.SearchIllustOptionsRequest{Word: searchWord})
	if err != nil {
		t.Fatalf("authenticated App search options failed: %v", err)
	}
	if searchOptions.Tools == nil {
		t.Fatal("search options returned a null or missing tools array")
	}

	resolution := canaryFilterCandidateValue(baseline.Illusts, canaryResolution)
	if resolution == "" {
		t.Fatal("authenticated App baseline returned no illustration in an official resolution bucket")
	}
	aspect := canaryFilterCandidateValue(baseline.Illusts, canaryAspectRatio)
	if aspect == "" {
		t.Fatal("authenticated App baseline returned no illustration with classifiable dimensions")
	}
	contentType := canaryFilterCandidateValue(baseline.Illusts, canaryContentType)
	if contentType == "" {
		t.Fatal("authenticated App baseline returned no illustration with a supported content type")
	}
	if !slices.ContainsFunc(baseline.Illusts, func(illust pixiv.Illust) bool { return illust.AIType != 2 }) {
		t.Fatal("authenticated App baseline returned no non-AI illustration candidate")
	}

	for _, search := range []struct {
		name     string
		filters  pixiv.SearchIllustFilters
		validate func(pixiv.Illust) bool
		want     string
	}{
		{
			name:     "resolution-" + resolution,
			filters:  pixiv.SearchIllustFilters{Resolution: pixiv.SearchResolution(resolution)},
			validate: func(illust pixiv.Illust) bool { return canaryResolution(illust) == resolution },
			want:     "official " + resolution + " resolution bucket",
		},
		{
			name:     "aspect-" + aspect,
			filters:  pixiv.SearchIllustFilters{AspectRatio: pixiv.SearchAspectRatio(aspect)},
			validate: func(illust pixiv.Illust) bool { return canaryAspectRatio(illust) == aspect },
			want:     aspect + " aspect ratio",
		},
		{
			name:     "content-type-" + contentType,
			filters:  pixiv.SearchIllustFilters{ContentType: pixiv.SearchContentType(contentType)},
			validate: func(illust pixiv.Illust) bool { return canaryContentType(illust) == contentType },
			want:     "content type " + contentType,
		},
		{
			name:     "exclude-ai",
			filters:  pixiv.SearchIllustFilters{AIMode: pixiv.SearchAIModeExclude},
			validate: func(illust pixiv.Illust) bool { return illust.AIType != 2 },
			want:     "ai_type != 2",
		},
	} {
		t.Run(search.name, func(t *testing.T) {
			result, err := searchCanaryNonempty(testCommandContext(t), client, pixiv.SearchIllustRequest{Word: searchWord, Filters: search.filters})
			if err != nil {
				t.Fatalf("search filter %s failed: %v", search.name, err)
			}
			if len(result.Illusts) == 0 {
				t.Fatalf("search filter %s returned no illustrations despite a matching baseline candidate", search.name)
			}
			for _, illust := range result.Illusts {
				if !search.validate(illust) {
					t.Fatalf("search filter %s returned illustration %d that does not satisfy %s", search.name, illust.ID, search.want)
				}
			}
		})
	}

	selectedTool, ok := canaryToolCandidate(searchOptions.Tools, baseline.Illusts)
	if !ok {
		t.Fatal("search options and authenticated baseline returned no shared tool candidate")
	}
	t.Run("tool", func(t *testing.T) {
		result, err := searchCanaryNonempty(testCommandContext(t), client, pixiv.SearchIllustRequest{
			Word: searchWord, Filters: pixiv.SearchIllustFilters{Tool: selectedTool},
		})
		if err != nil {
			t.Fatalf("tool-filter search failed: %v", err)
		}
		if len(result.Illusts) == 0 {
			t.Fatal("tool-filter search returned no illustrations despite a matching baseline candidate")
		}
		for _, illust := range result.Illusts {
			if !slices.Contains(illust.Tools, selectedTool) {
				t.Fatalf("tool-filter search returned illustration %d without the selected tool", illust.ID)
			}
		}
	})
}

func TestSearchCanaryNonemptyContinuesFilteredEmptyBatches(t *testing.T) {
	client := &scriptedSearchCanaryClient{results: []*pixiv.IllustListResult{
		{Illusts: []pixiv.Illust{}, NextCursor: "next-page"},
		{Illusts: []pixiv.Illust{{ID: 7}}},
	}}
	request := pixiv.SearchIllustRequest{
		Word: "miku",
		Filters: pixiv.SearchIllustFilters{
			Resolution: pixiv.SearchResolutionHigh,
			Tool:       "CLIP STUDIO PAINT",
		},
	}
	result, err := searchCanaryNonempty(context.Background(), client, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Illusts) != 1 || result.Illusts[0].ID != 7 {
		t.Fatalf("search result = %#v", result.Illusts)
	}
	if len(client.requests) != 2 || client.requests[1].Cursor != "next-page" {
		t.Fatalf("search requests = %#v", client.requests)
	}
	for _, got := range client.requests {
		if got.Word != request.Word || got.Filters != request.Filters {
			t.Fatalf("pagination changed filtered request: %#v", got)
		}
	}
}

func TestSearchCanaryNonemptyRejectsRepeatedCursor(t *testing.T) {
	client := &scriptedSearchCanaryClient{results: []*pixiv.IllustListResult{
		{Illusts: []pixiv.Illust{}, NextCursor: "same-page"},
		{Illusts: []pixiv.Illust{}, NextCursor: "same-page"},
	}}
	_, err := searchCanaryNonempty(context.Background(), client, pixiv.SearchIllustRequest{Word: "miku"})
	if err == nil || !strings.Contains(err.Error(), "repeated pagination cursor") {
		t.Fatalf("repeated cursor error = %v", err)
	}
}

type searchCanaryClient interface {
	SearchIllust(context.Context, pixiv.SearchIllustRequest) (*pixiv.IllustListResult, error)
}

// searchCanaryNonempty 按原请求自然消费本地筛选产生的空批次。它不添加任意
// 请求 cap；上游结束即返回，重复 opaque cursor 则明确失败以避免真实死循环。
func searchCanaryNonempty(ctx context.Context, client searchCanaryClient, request pixiv.SearchIllustRequest) (*pixiv.IllustListResult, error) {
	seen := make(map[pixiv.Cursor]struct{})
	for {
		if request.Cursor != "" {
			if _, ok := seen[request.Cursor]; ok {
				return nil, errors.New("search canary detected a repeated pagination cursor")
			}
			seen[request.Cursor] = struct{}{}
		}
		result, err := client.SearchIllust(ctx, request)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, errors.New("search canary received a nil result")
		}
		if len(result.Illusts) > 0 || result.NextCursor == "" {
			return result, nil
		}
		request.Cursor = result.NextCursor
	}
}

func assertCanaryEnvValue(t *testing.T, env []string, name, want string, wantCount int) {
	t.Helper()

	count := 0
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok && key == name {
			count++
			if value != want {
				t.Fatalf("%s = %q, want %q", name, value, want)
			}
		}
	}
	if count != wantCount {
		t.Fatalf("%s count = %d, want %d", name, count, wantCount)
	}
}

type handlerFailures struct {
	mu       sync.Mutex
	messages []string
}

func (f *handlerFailures) add(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
}

func (f *handlerFailures) requireEmpty(t *testing.T) {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.messages) > 0 {
		t.Fatalf("HTTP handler failures: %s", strings.Join(f.messages, "; "))
	}
}

type scriptedSearchCanaryClient struct {
	results  []*pixiv.IllustListResult
	requests []pixiv.SearchIllustRequest
}

func (c *scriptedSearchCanaryClient) SearchIllust(_ context.Context, request pixiv.SearchIllustRequest) (*pixiv.IllustListResult, error) {
	c.requests = append(c.requests, request)
	if len(c.results) == 0 {
		return nil, errors.New("unexpected extra search request")
	}
	result := c.results[0]
	c.results = c.results[1:]
	return result, nil
}

func TestCanaryToolCandidateStillUsesSharedClassifier(t *testing.T) {
	illusts := []pixiv.Illust{{Tools: []string{"A"}}, {Tools: []string{"B"}}}
	if tool, ok := canaryToolCandidate([]string{"B", "A"}, illusts); !ok || tool != "B" || !slices.Contains(illusts[1].Tools, tool) {
		t.Fatalf("tool candidate = %q, %v", tool, ok)
	}
}
