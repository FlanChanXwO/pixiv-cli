package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/stretchr/testify/require"
)

const pkceVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

func codeForm(t *testing.T, f authorizationFixture) url.Values {
	t.Helper()
	csrf, _ := f.consent(t)
	response := f.submit(t, url.Values{"csrf_token": {csrf}, "owner_secret": {f.secret}, "decision": {"allow"}})
	require.Equal(t, http.StatusFound, response.StatusCode)
	location, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	require.NotEmpty(t, location.Query().Get("code"))
	return url.Values{"grant_type": {"authorization_code"}, "code": {location.Query().Get("code")}, "client_id": {f.query.Get("client_id")}, "redirect_uri": {f.query.Get("redirect_uri")}, "resource": {f.query.Get("resource")}, "code_verifier": {pkceVerifier}}
}

func tokenRequest(t *testing.T, f authorizationFixture, form url.Values, status int) map[string]any {
	t.Helper()
	response, err := http.PostForm(f.server.URL+"/prefix/oauth/token", form)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, status, response.StatusCode)
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	var body map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	return body
}

func TestTokenCodeExchangePersistsHashesAndConsumesOnce(t *testing.T) {
	f := newAuthorizationFixture(t)
	form := codeForm(t, f)
	body := tokenRequest(t, f, form, http.StatusOK)
	require.Equal(t, "Bearer", body["token_type"])
	require.Equal(t, float64(3600), body["expires_in"])
	require.Equal(t, "mcp", body["scope"])
	access, ok := body["access_token"].(string)
	require.True(t, ok)
	refresh, ok := body["refresh_token"].(string)
	require.True(t, ok)
	require.GreaterOrEqual(t, len(access), 43)
	require.GreaterOrEqual(t, len(refresh), 43)
	require.True(t, access != refresh, "tokens must be independent")
	state, err := f.store.Read(t.Context())
	require.NoError(t, err)
	require.Len(t, state.Grants, 1)
	raw, err := os.ReadFile(f.store.Path)
	require.NoError(t, err)
	for _, secret := range []string{access, refresh, form.Get("code"), pkceVerifier} {
		require.False(t, bytes.Contains(raw, []byte(secret)), "state must not persist credentials")
	}
	failure := tokenRequest(t, f, form, http.StatusBadRequest)
	require.Equal(t, "invalid_grant", failure["error"])
}

func TestTokenRefreshRotatesAndReplayRevokesOnlyItsGrant(t *testing.T) {
	f := newAuthorizationFixture(t)
	initial := tokenRequest(t, f, codeForm(t, f), http.StatusOK)
	other := tokenRequest(t, f, codeForm(t, f), http.StatusOK)
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {initial["refresh_token"].(string)}, "client_id": {f.query.Get("client_id")}, "resource": {f.query.Get("resource")}}
	rotated := tokenRequest(t, f, form, http.StatusOK)
	require.True(t, initial["refresh_token"] != rotated["refresh_token"], "refresh must rotate")
	require.True(t, initial["access_token"] != rotated["access_token"], "access must rotate")
	require.Equal(t, "invalid_grant", tokenRequest(t, f, form, 400)["error"])
	form.Set("refresh_token", rotated["refresh_token"].(string))
	require.Equal(t, "invalid_grant", tokenRequest(t, f, form, 400)["error"])
	form.Set("refresh_token", "random-invalid-value")
	require.Equal(t, "invalid_grant", tokenRequest(t, f, form, 400)["error"])
	form.Set("refresh_token", other["refresh_token"].(string))
	tokenRequest(t, f, form, http.StatusOK)
}

func TestTokenRejectsMismatchedBindingsWithoutConsumingCode(t *testing.T) {
	for _, field := range []string{"code", "client_id", "redirect_uri", "resource", "scope", "code_verifier"} {
		t.Run(field, func(t *testing.T) {
			f := newAuthorizationFixture(t)
			good := codeForm(t, f)
			bad, err := url.ParseQuery(good.Encode())
			require.NoError(t, err)
			bad.Set(field, "wrong-binding")
			require.Equal(t, "invalid_grant", tokenRequest(t, f, bad, 400)["error"])
			tokenRequest(t, f, good, 200)
		})
	}
}

func TestTokenRefreshRejectsMismatchedBindingsWithoutRotating(t *testing.T) {
	for _, field := range []string{"client_id", "resource", "scope"} {
		t.Run(field, func(t *testing.T) {
			f := newAuthorizationFixture(t)
			pair := tokenRequest(t, f, codeForm(t, f), 200)
			good := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {pair["refresh_token"].(string)}, "client_id": {f.query.Get("client_id")}, "resource": {f.query.Get("resource")}}
			bad, err := url.ParseQuery(good.Encode())
			require.NoError(t, err)
			bad.Set(field, "wrong-binding")
			require.Equal(t, "invalid_grant", tokenRequest(t, f, bad, 400)["error"])
			tokenRequest(t, f, good, 200)
		})
	}
}

// 直接走 HTTP handler，避免 synctest 的假时钟与真实网络混用。
func directCode(t *testing.T) (*auth.Handler, auth.Store, url.Values) {
	t.Helper()
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	secret, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	h, err := auth.NewHandler("http://instance.test/prefix", store)
	require.NoError(t, err)
	request := httptest.NewRequest("POST", "/prefix/oauth/register", strings.NewReader(`{"redirect_uris":["https://client.test/callback"]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	require.Equal(t, 201, response.Code)
	var client map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &client))
	form := url.Values{"client_id": {client["client_id"].(string)}, "redirect_uri": {"https://client.test/callback"}, "response_type": {"code"}, "resource": {"http://instance.test/prefix/mcp"}, "scope": {"mcp"}, "code_challenge_method": {"S256"}, "code_challenge": {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest("GET", "/prefix/oauth/authorize?"+form.Encode(), nil))
	require.Equal(t, 200, response.Code)
	csrf := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(response.Body.String())
	require.Len(t, csrf, 2)
	values := url.Values{"csrf_token": {csrf[1]}, "owner_secret": {secret}, "decision": {"allow"}}
	request = httptest.NewRequest("POST", "/prefix/oauth/authorize", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	request.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	require.Equal(t, 302, response.Code)
	callback, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	require.NotEmpty(t, callback.Query().Get("code"))
	return h, store, url.Values{"grant_type": {"authorization_code"}, "code": {callback.Query().Get("code")}, "client_id": {form.Get("client_id")}, "redirect_uri": {form.Get("redirect_uri")}, "resource": {form.Get("resource")}, "code_verifier": {pkceVerifier}}
}

func directToken(h http.Handler, form url.Values) *httptest.ResponseRecorder {
	request := httptest.NewRequest("POST", "/prefix/oauth/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func decodedToken(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, 200, response.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	return body
}

func TestTokenCodeDeadline(t *testing.T) {
	for _, tc := range []struct {
		delay  time.Duration
		status int
	}{{10*time.Minute - time.Nanosecond, 200}, {10 * time.Minute, 400}} {
		t.Run(tc.delay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h, _, form := directCode(t)
				time.Sleep(tc.delay)
				require.Equal(t, tc.status, directToken(h, form).Code)
			})
		})
	}
}

func TestTokenRestartKeepsRefreshButLosesPendingCodes(t *testing.T) {
	h, store, form := directCode(t)
	restarted, err := auth.NewHandler("http://instance.test/prefix", store)
	require.NoError(t, err)
	require.Equal(t, 400, directToken(restarted, form).Code)
	pair := decodedToken(t, directToken(h, form))
	refresh := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {pair["refresh_token"].(string)}, "client_id": {form.Get("client_id")}, "resource": {form.Get("resource")}}
	decodedToken(t, directToken(restarted, refresh))
	require.Equal(t, 400, directToken(h, refresh).Code)
}

func TestTokenRefreshHasNoIdleDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, form := directCode(t)
		pair := decodedToken(t, directToken(h, form))
		time.Sleep(100 * 365 * 24 * time.Hour)
		decodedToken(t, directToken(h, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {pair["refresh_token"].(string)}, "client_id": {form.Get("client_id")}, "resource": {form.Get("resource")}}))
	})
}

func TestTokenConcurrentExchangeHasOneWinner(t *testing.T) {
	for _, kind := range []string{"authorization_code", "refresh_token"} {
		t.Run(kind, func(t *testing.T) {
			h, store, form := directCode(t)
			second := h
			if kind == "refresh_token" {
				pair := decodedToken(t, directToken(h, form))
				form = url.Values{"grant_type": {kind}, "refresh_token": {pair["refresh_token"].(string)}, "client_id": {form.Get("client_id")}, "resource": {form.Get("resource")}}
				var err error
				second, err = auth.NewHandler("http://instance.test/prefix", store)
				require.NoError(t, err)
			}
			start := make(chan struct{})
			results := make(chan *httptest.ResponseRecorder, 2)
			for _, handler := range []*auth.Handler{h, second} {
				go func() { <-start; results <- directToken(handler, form) }()
			}
			close(start)
			a, b := <-results, <-results
			require.ElementsMatch(t, []int{200, 400}, []int{a.Code, b.Code})
			if kind == "refresh_token" {
				winner := a
				if winner.Code != 200 {
					winner = b
				}
				pair := decodedToken(t, winner)
				protected := h.RequireBearer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
				require.Equal(t, 401, bearerRequest(protected, pair["access_token"].(string)).Code)
			}
		})
	}
}

func TestTokenResetRacesCannotResurrectGrants(t *testing.T) {
	for _, kind := range []string{"authorization_code", "refresh_token"} {
		t.Run(kind, func(t *testing.T) {
			h, store, form := directCode(t)
			if kind == "refresh_token" {
				pair := decodedToken(t, directToken(h, form))
				form = url.Values{"grant_type": {kind}, "refresh_token": {pair["refresh_token"].(string)}, "client_id": {form.Get("client_id")}, "resource": {form.Get("resource")}}
			}
			start := make(chan struct{})
			result := make(chan *httptest.ResponseRecorder, 1)
			reset := make(chan error, 1)
			go func() { <-start; result <- directToken(h, form) }()
			go func() { <-start; _, err := store.Init(t.Context(), true); reset <- err }()
			close(start)
			response := <-result
			require.NoError(t, <-reset)
			require.Contains(t, []int{200, 400}, response.Code)
			state, err := store.Read(t.Context())
			require.NoError(t, err)
			require.Empty(t, state.Grants)
			require.Equal(t, 400, directToken(h, form).Code)
			if response.Code == 200 {
				pair := decodedToken(t, response)
				protected := h.RequireBearer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
				require.Equal(t, 401, bearerRequest(protected, pair["access_token"].(string)).Code)
			}
		})
	}
}

func TestTokenRefreshRejectsAnotherCanonicalResource(t *testing.T) {
	h, store, form := directCode(t)
	pair := decodedToken(t, directToken(h, form))
	other, err := auth.NewHandler("http://other.test/prefix", store)
	require.NoError(t, err)
	refresh := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {pair["refresh_token"].(string)}, "client_id": {form.Get("client_id")}, "resource": {form.Get("resource")}}
	require.Equal(t, 400, directToken(other, refresh).Code)
	decodedToken(t, directToken(h, refresh))
}

func TestTokenErrorsUseOAuthJSONWithoutSecrets(t *testing.T) {
	h, _, form := directCode(t)
	for _, key := range []string{"grant_type", "client_id", "resource", "code", "redirect_uri", "code_verifier"} {
		t.Run("missing_"+key, func(t *testing.T) {
			incomplete, err := url.ParseQuery(form.Encode())
			require.NoError(t, err)
			incomplete.Del(key)
			response := directToken(h, incomplete)
			require.Equal(t, 400, response.Code)
			require.JSONEq(t, `{"error":"invalid_request"}`, response.Body.String())
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		})
	}
	for _, tc := range []struct {
		method, target, content, body, failure string
		status                                 int
	}{
		{"GET", "/prefix/oauth/token", "", "", "invalid_request", 405},
		{"POST", "/prefix/oauth/token", "application/json", `{"secret":"never-echo"}`, "invalid_request", 400},
		{"POST", "/prefix/oauth/token?code=never-echo", "application/x-www-form-urlencoded", form.Encode(), "invalid_request", 400},
		{"POST", "/prefix/oauth/token", "application/x-www-form-urlencoded", "code=%zz", "invalid_request", 400},
		{"POST", "/prefix/oauth/token", "application/x-www-form-urlencoded", form.Encode() + "&code=never-echo", "invalid_request", 400},
		{"POST", "/prefix/oauth/token", "application/x-www-form-urlencoded", "grant_type=password", "unsupported_grant_type", 400},
	} {
		request := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
		request.Header.Set("Content-Type", tc.content)
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		require.Equal(t, tc.status, response.Code)
		require.JSONEq(t, `{"error":"`+tc.failure+`"}`, response.Body.String())
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	}
	decodedToken(t, directToken(h, form))
}
