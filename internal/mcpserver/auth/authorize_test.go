package auth_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/stretchr/testify/require"
)

func TestAuthorizeShowsExplicitOwnerConsent(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	handler, err := auth.NewHandler("http://instance.test/prefix", store)
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	defer server.Close()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	browser := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := browser.Post(server.URL+"/prefix/oauth/register", "application/json", strings.NewReader(`{"client_name":"<img src=x onerror=alert(1)>","redirect_uris":["https://client.test/callback?retained=1"]}`))
	require.NoError(t, err)
	var registered map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&registered))
	response.Body.Close()
	query := url.Values{"client_id": {registered["client_id"].(string)}, "redirect_uri": {"https://client.test/callback?retained=1"}, "response_type": {"code"}, "resource": {"http://instance.test/prefix/mcp"}, "scope": {"mcp"}, "state": {"opaque state + &"}, "code_challenge_method": {"S256"}, "code_challenge": {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}}
	response, err = browser.Get(server.URL + "/prefix/oauth/authorize?" + query.Encode())
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `type="password"`)
	require.Contains(t, string(body), `autocomplete="current-password"`)
	require.Contains(t, string(body), `name="csrf_token"`)
	require.Contains(t, string(body), `value="allow"`)
	require.Contains(t, string(body), `value="deny"`)
	require.Contains(t, string(body), "Unverified client name")
	require.Contains(t, string(body), "https://client.test")
	require.NotContains(t, string(body), "<img")
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	require.Equal(t, "strict-origin", response.Header.Get("Referrer-Policy"))
	require.Contains(t, response.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'")
	require.Contains(t, response.Header.Get("Content-Security-Policy"), "form-action 'self' https://client.test;")
	require.Empty(t, response.Header.Get("Location"))
	require.Len(t, response.Cookies(), 1)
	cookie := response.Cookies()[0]
	require.True(t, cookie.HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	require.False(t, cookie.Secure)
	require.Equal(t, "/prefix/oauth", cookie.Path)
	require.Empty(t, cookie.Domain)
}

type authorizationFixture struct {
	store   auth.Store
	secret  string
	server  *httptest.Server
	browser *http.Client
	query   url.Values
}

func newAuthorizationFixture(t *testing.T) authorizationFixture {
	t.Helper()
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	secret, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	handler, err := auth.NewHandler("http://instance.test/prefix", store)
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	browser := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := browser.Post(server.URL+"/prefix/oauth/register", "application/json", strings.NewReader(`{"client_name":"Test connector","redirect_uris":["https://client.test/callback?retained=1"]}`))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var registered map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&registered))
	return authorizationFixture{store: store, secret: secret, server: server, browser: browser, query: url.Values{"client_id": {registered["client_id"].(string)}, "redirect_uri": {"https://client.test/callback?retained=1"}, "response_type": {"code"}, "resource": {"http://instance.test/prefix/mcp"}, "scope": {"mcp"}, "state": {"opaque state + &"}, "code_challenge_method": {"S256"}, "code_challenge": {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}}}
}

func (f authorizationFixture) consent(t *testing.T) (string, string) {
	t.Helper()
	response, err := f.browser.Get(f.server.URL + "/prefix/oauth/authorize?" + f.query.Encode())
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	matches := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(string(body))
	require.Len(t, matches, 2)
	require.NotEqual(t, f.query.Get("state"), matches[1])
	return matches[1], string(body)
}

func (f authorizationFixture) submit(t *testing.T, values url.Values) *http.Response {
	t.Helper()
	response, err := f.browser.PostForm(f.server.URL+"/prefix/oauth/authorize", values)
	require.NoError(t, err)
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func TestAuthorizeIssuesCodeOnlyAfterOwnerAndExplicitConsent(t *testing.T) {
	f := newAuthorizationFixture(t)
	before, err := os.ReadFile(f.store.Path)
	require.NoError(t, err)
	csrf, _ := f.consent(t)
	endpoint, err := url.Parse(f.server.URL + "/prefix/oauth/authorize")
	require.NoError(t, err)
	oldCookie := f.browser.Jar.Cookies(endpoint)[0]
	response := f.submit(t, url.Values{"csrf_token": {csrf}, "owner_secret": {f.secret}, "decision": {"allow"}})
	require.Equal(t, http.StatusFound, response.StatusCode)
	redirect, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "https", redirect.Scheme)
	require.Equal(t, "client.test", redirect.Host)
	require.Equal(t, "/callback", redirect.Path)
	require.Equal(t, "1", redirect.Query().Get("retained"))
	require.Equal(t, f.query.Get("state"), redirect.Query().Get("state"))
	require.Equal(t, "http://instance.test/prefix", redirect.Query().Get("iss"))
	code := redirect.Query().Get("code")
	require.NotEmpty(t, code)
	require.True(t, code != f.secret)
	require.NotEqual(t, oldCookie.Value, f.browser.Jar.Cookies(endpoint)[0].Value)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Empty(t, body)
	after, err := os.ReadFile(f.store.Path)
	require.NoError(t, err)
	require.Equal(t, before, after, "authorization codes and sessions must remain in memory")
	require.Equal(t, http.StatusForbidden, f.submit(t, url.Values{"csrf_token": {csrf}, "decision": {"allow"}}).StatusCode)
	next, page := f.consent(t)
	require.NotContains(t, page, `type="password"`)
	require.Contains(t, page, `value="allow"`)
	require.Contains(t, page, `value="deny"`)
	require.NotContains(t, page, code)
	require.NotEqual(t, csrf, next)
	response = f.submit(t, url.Values{"csrf_token": {next}, "decision": {"deny"}})
	require.Equal(t, http.StatusFound, response.StatusCode)
	denied, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "access_denied", denied.Query().Get("error"))
	require.Empty(t, denied.Query().Get("code"))
	require.Equal(t, f.query.Get("state"), denied.Query().Get("state"))
	require.Equal(t, "http://instance.test/prefix", denied.Query().Get("iss"))
}

func TestAuthorizeValidatesClientRedirectAndPKCE(t *testing.T) {
	for _, tc := range []struct {
		key, value, failure string
		local               bool
	}{
		{"client_id", "unknown", "invalid_request", true},
		{"redirect_uri", "https://attacker.test/callback", "invalid_request", true},
		{"redirect_uri", "https://client.test/callback?retained=2", "invalid_request", true},
		{"redirect_uri", "", "invalid_request", true},
		{"response_type", "token", "unsupported_response_type", false},
		{"resource", "http://attacker.test/mcp", "invalid_target", false},
		{"scope", "admin", "invalid_scope", false},
		{"code_challenge_method", "plain", "invalid_request", false},
		{"code_challenge_method", "", "invalid_request", false},
		{"code_challenge", "short", "invalid_request", false},
		{"code_challenge", strings.Repeat("_", 43), "invalid_request", false},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			f := newAuthorizationFixture(t)
			f.query.Set(tc.key, tc.value)
			response, err := f.browser.Get(f.server.URL + "/prefix/oauth/authorize?" + f.query.Encode())
			require.NoError(t, err)
			defer response.Body.Close()
			if tc.local {
				require.Equal(t, http.StatusBadRequest, response.StatusCode)
				require.Empty(t, response.Header.Get("Location"))
			} else {
				require.Equal(t, http.StatusFound, response.StatusCode)
				redirect, err := url.Parse(response.Header.Get("Location"))
				require.NoError(t, err)
				require.Equal(t, "client.test", redirect.Host)
				require.Equal(t, tc.failure, redirect.Query().Get("error"))
				require.Empty(t, redirect.Query().Get("code"))
				require.Equal(t, "http://instance.test/prefix", redirect.Query().Get("iss"))
				require.Equal(t, f.query.Get("state"), redirect.Query().Get("state"))
			}
		})
	}
	for _, key := range []string{"client_id", "redirect_uri", "state", "resource", "response_type", "code_challenge"} {
		t.Run("duplicate_"+key, func(t *testing.T) {
			f := newAuthorizationFixture(t)
			f.query.Add(key, f.query.Get(key))
			response, err := f.browser.Get(f.server.URL + "/prefix/oauth/authorize?" + f.query.Encode())
			require.NoError(t, err)
			defer response.Body.Close()
			require.NotEqual(t, http.StatusOK, response.StatusCode)
			require.Empty(t, response.Cookies())
		})
	}
}

func TestAuthorizeRejectsForgedOrAmbiguousConsent(t *testing.T) {
	for _, attack := range []string{"wrong owner", "missing csrf", "state as csrf", "duplicate decision", "override redirect", "foreign origin", "query override"} {
		t.Run(attack, func(t *testing.T) {
			f := newAuthorizationFixture(t)
			csrf, _ := f.consent(t)
			values := url.Values{"csrf_token": {csrf}, "owner_secret": {f.secret}, "decision": {"allow"}}
			target := f.server.URL + "/prefix/oauth/authorize"
			switch attack {
			case "wrong owner":
				values.Set("owner_secret", "wrong")
			case "missing csrf":
				values.Del("csrf_token")
			case "state as csrf":
				values.Set("csrf_token", f.query.Get("state"))
			case "duplicate decision":
				values.Add("decision", "deny")
			case "override redirect":
				values.Set("redirect_uri", "https://attacker.test")
			case "query override":
				target += "?decision=allow"
			}
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(values.Encode()))
			require.NoError(t, err)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if attack == "foreign origin" {
				request.Header.Set("Origin", "https://attacker.test")
				request.Header.Set("X-Forwarded-Host", "attacker.test")
			}
			response, err := f.browser.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.GreaterOrEqual(t, response.StatusCode, 400)
			require.Empty(t, response.Header.Get("Location"))
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.False(t, strings.Contains(string(body), f.secret))
		})
	}
}

func TestAuthorizeSessionResetRestartAndBrowserBinding(t *testing.T) {
	f := newAuthorizationFixture(t)
	csrf, _ := f.consent(t)
	otherJar, err := cookiejar.New(nil)
	require.NoError(t, err)
	other := f
	other.browser = &http.Client{Jar: otherJar, CheckRedirect: f.browser.CheckRedirect}
	_, _ = other.consent(t)
	require.Equal(t, http.StatusForbidden, other.submit(t, url.Values{"csrf_token": {csrf}, "owner_secret": {f.secret}, "decision": {"allow"}}).StatusCode)
	require.Equal(t, http.StatusFound, f.submit(t, url.Values{"csrf_token": {csrf}, "owner_secret": {f.secret}, "decision": {"allow"}}).StatusCode)
	pending, _ := f.consent(t)
	newSecret, err := f.store.Init(t.Context(), true)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, f.submit(t, url.Values{"csrf_token": {pending}, "decision": {"allow"}}).StatusCode)
	csrf, page := f.consent(t)
	require.Contains(t, page, `type="password"`)
	require.Equal(t, http.StatusForbidden, f.submit(t, url.Values{"csrf_token": {csrf}, "owner_secret": {f.secret}, "decision": {"allow"}}).StatusCode)
	csrf, _ = f.consent(t)
	require.Equal(t, http.StatusFound, f.submit(t, url.Values{"csrf_token": {csrf}, "owner_secret": {newSecret}, "decision": {"allow"}}).StatusCode)
	handler, err := auth.NewHandler("http://instance.test/prefix", f.store)
	require.NoError(t, err)
	restarted := httptest.NewServer(handler)
	defer restarted.Close()
	f.server = restarted
	_, page = f.consent(t)
	require.Contains(t, page, `type="password"`, "a new server must not recover owner browser sessions")
}

func TestAuthorizeConcurrentConsentIssuesOnlyOneCode(t *testing.T) {
	f := newAuthorizationFixture(t)
	csrf, _ := f.consent(t)
	target, err := url.Parse(f.server.URL + "/prefix/oauth/authorize")
	require.NoError(t, err)
	cookie := f.browser.Jar.Cookies(target)[0]
	results := make(chan int, 8)
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			values := url.Values{"csrf_token": {csrf}, "owner_secret": {f.secret}, "decision": {"allow"}}
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target.String(), strings.NewReader(values.Encode()))
			require.NoError(t, err)
			request.AddCookie(cookie)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response, err := (&http.Client{CheckRedirect: f.browser.CheckRedirect}).Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			results <- response.StatusCode
		}()
	}
	workers.Wait()
	close(results)
	succeeded, rejected := 0, 0
	for status := range results {
		switch status {
		case http.StatusFound:
			succeeded++
		case http.StatusForbidden:
			rejected++
		default:
			t.Errorf("unexpected status %d", status)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 7, rejected)
}

func TestAuthorizeUsesCanonicalCookieSecurityAndRechecksRegistration(t *testing.T) {
	f := newAuthorizationFixture(t)
	handler, err := auth.NewHandler("https://instance.test/prefix", f.store)
	require.NoError(t, err)
	secureQuery, err := url.ParseQuery(f.query.Encode())
	require.NoError(t, err)
	secureQuery.Set("resource", "https://instance.test/prefix/mcp")
	request := httptest.NewRequest(http.MethodGet, "http://attacker.test/prefix/oauth/authorize?"+secureQuery.Encode(), nil)
	request.Header.Set("X-Forwarded-Proto", "http")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, recorder.Result().Cookies()[0].Secure)
	csrf, _ := f.consent(t)
	state, err := f.store.Read(t.Context())
	require.NoError(t, err)
	delete(state.Clients, f.query.Get("client_id"))
	body, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f.store.Path, body, 0600))
	response := f.submit(t, url.Values{"csrf_token": {csrf}, "owner_secret": {f.secret}, "decision": {"allow"}})
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.Empty(t, response.Header.Get("Location"))
}

func TestAuthorizeComparesBrowserOrigins(t *testing.T) {
	for _, tc := range []struct {
		origin string
		status int
	}{
		{"http://instance.test", http.StatusFound},
		{"http://INSTANCE.TEST:80", http.StatusFound},
		{"http://instance.test:8080", http.StatusForbidden},
		{"https://instance.test", http.StatusForbidden},
		{"null", http.StatusForbidden},
		{"http://instance.test/", http.StatusForbidden},
		{"http://instance.test#", http.StatusForbidden},
		{"http://instance.test?", http.StatusForbidden},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			f := newAuthorizationFixture(t)
			handler, err := auth.NewHandler("http://INSTANCE.TEST:80/prefix", f.store)
			require.NoError(t, err)
			server := httptest.NewServer(handler)
			defer server.Close()
			f.server = server
			f.query.Set("resource", "http://INSTANCE.TEST:80/prefix/mcp")
			csrf, _ := f.consent(t)
			values := url.Values{"csrf_token": {csrf}, "owner_secret": {f.secret}, "decision": {"allow"}}
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/prefix/oauth/authorize", strings.NewReader(values.Encode()))
			require.NoError(t, err)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", tc.origin)
			response, err := f.browser.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, tc.status, response.StatusCode)
		})
	}
}

func TestAuthorizeRejectsAmbiguousCallbackQuery(t *testing.T) {
	for _, suffix := range []string{"code=old", "error=old", "state=old", "iss=old", "error_description=old", "error_uri=old", "retained=1;code=old"} {
		t.Run(suffix, func(t *testing.T) {
			f := newAuthorizationFixture(t)
			state, err := f.store.Read(t.Context())
			require.NoError(t, err)
			redirect := "https://client.test/callback?" + suffix
			client := state.Clients[f.query.Get("client_id")]
			client.RedirectURIs = []string{redirect}
			state.Clients[f.query.Get("client_id")] = client
			raw, err := json.Marshal(state)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(f.store.Path, raw, 0600))
			f.query.Set("redirect_uri", redirect)
			response, err := f.browser.Get(f.server.URL + "/prefix/oauth/authorize?" + f.query.Encode())
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			require.Empty(t, response.Header.Get("Location"))
		})
	}
}

func TestAuthorizeCookieUsesEscapedPathPrefix(t *testing.T) {
	f := newAuthorizationFixture(t)
	handler, err := auth.NewHandler("http://instance.test/my%20instance", f.store)
	require.NoError(t, err)
	f.query.Set("resource", "http://instance.test/my%20instance/mcp")
	request := httptest.NewRequest(http.MethodGet, "http://instance.test/my%20instance/oauth/authorize?"+f.query.Encode(), nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "/my%20instance/oauth", recorder.Result().Cookies()[0].Path)
	require.Contains(t, recorder.Body.String(), `action="/my%20instance/oauth/authorize"`)
}
