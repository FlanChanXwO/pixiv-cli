package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryUsesCanonicalURLs(t *testing.T) {
	for _, tc := range []struct{ base, issuer, prefix string }{
		{"https://example.test", "https://example.test", ""},
		{"https://example.test/pixiv/team/", "https://example.test/pixiv/team", "/pixiv/team"},
		{"http://127.0.0.1:8123/local", "http://127.0.0.1:8123/local", "/local"},
	} {
		t.Run(tc.base, func(t *testing.T) {
			handler, err := auth.NewHandler(tc.base, auth.Store{Path: filepath.Join(t.TempDir(), "state.json")})
			require.NoError(t, err)
			server := httptest.NewServer(handler)
			defer server.Close()
			get := func(route string) map[string]any {
				t.Helper()
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+route, nil)
				require.NoError(t, err)
				req.Host = "attacker.invalid"
				req.Header.Set("X-Forwarded-Host", "attacker.invalid")
				req.Header.Set("X-Forwarded-Proto", "http")
				resp, err := server.Client().Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode, route)
				require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
				var result map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
				return result
			}
			prm := get("/.well-known/oauth-protected-resource")
			require.Equal(t, prm, get("/.well-known/oauth-protected-resource"+tc.prefix+"/mcp"))
			require.Equal(t, tc.issuer+"/mcp", prm["resource"])
			require.Equal(t, []any{tc.issuer}, prm["authorization_servers"])
			meta := get("/.well-known/oauth-authorization-server" + tc.prefix)
			require.Equal(t, map[string]any{
				"issuer":                                tc.issuer,
				"authorization_endpoint":                tc.issuer + "/oauth/authorize",
				"token_endpoint":                        tc.issuer + "/oauth/token",
				"registration_endpoint":                 tc.issuer + "/oauth/register",
				"response_types_supported":              []any{"code"},
				"grant_types_supported":                 []any{"authorization_code", "refresh_token"},
				"code_challenge_methods_supported":      []any{"S256"},
				"token_endpoint_auth_methods_supported": []any{"none"},
				"scopes_supported":                      []any{"mcp"},
			}, meta)
		})
	}
}

func TestDiscoveryRejectsInvalidConfigurationAndMethods(t *testing.T) {
	for _, base := range []string{"", "/relative", "ftp://example.test", "https://", "https://user:secret@example.test", "https://example.test?q=x", "https://example.test?", "https://example.test#", "https://example.test/#fragment", "http://example.test:99999", "http://example.test:", `https://example.test/a\b`, "https://[not-an-ipv6-address]/prefix", "https://example.test/a/../b", "https://example.test/a/%2e%2e/b"} {
		t.Run(base, func(t *testing.T) {
			handler, err := auth.NewHandler(base, auth.Store{Path: filepath.Join(t.TempDir(), "state.json")})
			require.Error(t, err)
			require.Nil(t, handler)
			require.NotContains(t, err.Error(), "secret")
		})
	}
	handler, err := auth.NewHandler("https://example.test", auth.Store{})
	require.Error(t, err)
	require.Nil(t, handler)
	handler, err = auth.NewHandler("https://example.test", auth.Store{Path: filepath.Join(t.TempDir(), "state.json")})
	require.NoError(t, err)
	for _, route := range []string{"/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, route, nil))
		require.Equal(t, http.StatusMethodNotAllowed, response.Code, route)
	}
}

func TestRegistrationPersistsPublicClient(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	handler, err := auth.NewHandler("https://example.test/prefix", store)
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	defer server.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/prefix/oauth/register", strings.NewReader(`{"client_name":"test connector","redirect_uris":["https://client.test/callback?state=a%2Fb","http://127.0.0.1:4321/callback","com.example.app:/oauth2redirect"]}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusCreated, response.StatusCode)
	require.Equal(t, "application/json", response.Header.Get("Content-Type"))
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	var result map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	id, ok := result["client_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)
	require.Equal(t, "none", result["token_endpoint_auth_method"])
	require.NotContains(t, result, "client_secret")
	require.Equal(t, []any{"authorization_code", "refresh_token"}, result["grant_types"])
	require.Equal(t, []any{"code"}, result["response_types"])
	require.Equal(t, "mcp", result["scope"])
	reopened, err := (auth.Store{Path: store.Path}).Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, auth.Client{ClientName: "test connector", RedirectURIs: []string{"https://client.test/callback?state=a%2Fb", "http://127.0.0.1:4321/callback", "com.example.app:/oauth2redirect"}}, reopened.Clients[id])
	require.Equal(t, result["redirect_uris"], []any{"https://client.test/callback?state=a%2Fb", "http://127.0.0.1:4321/callback", "com.example.app:/oauth2redirect"})
	_, err = store.Init(t.Context(), true)
	require.NoError(t, err)
	reset, err := (auth.Store{Path: store.Path}).Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, reopened.Clients, reset.Clients)
}

func registrationRequest(t *testing.T, server *httptest.Server, method, body, contentType string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, server.URL+"/oauth/register", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	response, err := server.Client().Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, "application/json", response.Header.Get("Content-Type"))
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	var result map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	return response.StatusCode, result
}

func TestRegistrationRejectsInvalidMetadataWithoutMutation(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	before, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	handler, err := auth.NewHandler("https://example.test", store)
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	defer server.Close()
	for _, tc := range []struct{ name, body, code string }{
		{"syntax", `{`, "invalid_client_metadata"},
		{"trailing JSON", `{"redirect_uris":["https://client.test/cb"]} {}`, "invalid_client_metadata"},
		{"missing redirects", `{}`, "invalid_redirect_uri"},
		{"empty redirects", `{"redirect_uris":[]}`, "invalid_redirect_uri"},
		{"null redirects", `{"redirect_uris":null}`, "invalid_redirect_uri"},
		{"secret method", `{"redirect_uris":["https://client.test/cb"],"token_endpoint_auth_method":"client_secret_basic"}`, "invalid_client_metadata"},
		{"empty method", `{"redirect_uris":["https://client.test/cb"],"token_endpoint_auth_method":""}`, "invalid_client_metadata"},
		{"null method", `{"redirect_uris":["https://client.test/cb"],"token_endpoint_auth_method":null}`, "invalid_client_metadata"},
		{"unsupported grants", `{"redirect_uris":["https://client.test/cb"],"grant_types":["client_credentials"]}`, "invalid_client_metadata"},
		{"empty grants", `{"redirect_uris":["https://client.test/cb"],"grant_types":[]}`, "invalid_client_metadata"},
		{"unsupported response", `{"redirect_uris":["https://client.test/cb"],"response_types":["token"]}`, "invalid_client_metadata"},
		{"unsupported scope", `{"redirect_uris":["https://client.test/cb"],"scope":"admin"}`, "invalid_client_metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, result := registrationRequest(t, server, http.MethodPost, tc.body, "application/json")
			require.Equal(t, http.StatusBadRequest, status)
			require.Equal(t, tc.code, result["error"])
			require.NotContains(t, result, "client_id")
		})
	}
	for _, redirect := range []string{"", "/callback", "https://client.test/cb#secret", "https://client.test/cb#", "javascript:alert(1)", "data:text/html,hi", "file:///tmp/cb", "myapp:/callback", "http://client.test/cb", "http://127.0.0.1.attacker.test/cb", "http://localhost.attacker.test/cb", "https://", "https://user:secret@client.test/cb", "https://client.test:99999/cb", "https://client.test/a b", `https://client.test/a\b`, "https://[not-an-ipv6-address]/cb"} {
		t.Run(redirect, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"redirect_uris": []string{redirect}})
			require.NoError(t, err)
			status, result := registrationRequest(t, server, http.MethodPost, string(body), "application/json")
			require.Equal(t, http.StatusBadRequest, status)
			require.Equal(t, "invalid_redirect_uri", result["error"])
			require.NotContains(t, result, "client_id")
		})
	}
	status, result := registrationRequest(t, server, http.MethodGet, `{"redirect_uris":["https://client.test/cb"]}`, "application/json")
	require.Equal(t, http.StatusMethodNotAllowed, status)
	require.Equal(t, "invalid_request", result["error"])
	status, result = registrationRequest(t, server, http.MethodPost, `{"redirect_uris":["https://client.test/cb"]}`, "text/plain")
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "invalid_client_metadata", result["error"])
	after, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
}

func TestRegistrationAcceptsNativeClientsWithoutFetchingMetadata(t *testing.T) {
	var fetches atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fetches.Add(1) }))
	defer remote.Close()
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	handler, err := auth.NewHandler("https://example.test", store)
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	defer server.Close()
	redirects := []string{"https://client.test/cb?x=%2F", "http://127.0.0.1:8321/cb", "http://[::1]:8321/cb", "http://localhost:8321/cb", "com.example.app:/oauth2redirect"}
	body, err := json.Marshal(map[string]any{"redirect_uris": redirects, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code"}, "response_types": []string{"code"}, "scope": "mcp", "logo_uri": remote.URL, "client_uri": remote.URL, "jwks_uri": remote.URL, "client_secret": "do-not-persist"})
	require.NoError(t, err)
	status, result := registrationRequest(t, server, http.MethodPost, string(body), "application/json; charset=utf-8")
	require.Equal(t, http.StatusCreated, status)
	require.Zero(t, fetches.Load())
	require.NotContains(t, result, "client_secret")
	state, err := (auth.Store{Path: store.Path}).Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, redirects, state.Clients[result["client_id"].(string)].RedirectURIs)
	raw, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "do-not-persist")
	require.NotContains(t, string(raw), remote.URL)
}

func TestRegistrationPreservesStateAcrossConcurrentWritersAndReset(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	state, err := store.Read(t.Context())
	require.NoError(t, err)
	state.SelectedPixivUserID = 42
	state.Clients["existing"] = auth.Client{RedirectURIs: []string{"https://existing.test/cb"}}
	state.Grants["grant"] = auth.Grant{ClientID: "existing", Resource: "https://example.test/mcp", Scope: "mcp", RefreshHash: strings.Repeat("a", 64), AccessTokens: map[string]time.Time{}}
	raw, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.Path, raw, 0600))
	handler, err := auth.NewHandler("https://example.test", store)
	require.NoError(t, err)
	server := httptest.NewServer(handler)
	defer server.Close()
	status, _ := registrationRequest(t, server, http.MethodPost, `{"redirect_uris":["https://client.test/cb"]}`, "application/json")
	require.Equal(t, http.StatusCreated, status)
	registered, err := store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, state.OwnerVerifier, registered.OwnerVerifier)
	require.Equal(t, state.Grants, registered.Grants)
	require.Equal(t, state.SelectedPixivUserID, registered.SelectedPixivUserID)

	var writers sync.WaitGroup
	for i := 0; i < 8; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			independent := auth.Store{Path: store.Path}
			handler, err := auth.NewHandler("https://example.test", independent)
			require.NoError(t, err)
			server := httptest.NewServer(handler)
			defer server.Close()
			status, _ := registrationRequest(t, server, http.MethodPost, `{"redirect_uris":["https://client.test/cb"]}`, "application/json")
			require.Equal(t, http.StatusCreated, status)
			_, err = independent.Init(t.Context(), true)
			require.NoError(t, err)
		}()
	}
	writers.Wait()
	reopened, err := (auth.Store{Path: store.Path}).Read(t.Context())
	require.NoError(t, err)
	require.Len(t, reopened.Clients, 10)
	require.Equal(t, state.Clients["existing"], reopened.Clients["existing"])
	require.Equal(t, state.SelectedPixivUserID, reopened.SelectedPixivUserID)
	require.NotEqual(t, state.OwnerVerifier, reopened.OwnerVerifier)
	require.Empty(t, reopened.Grants)
}

func TestRegistrationFailsClosedOnUnavailableState(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
		if corrupt {
			require.NoError(t, os.WriteFile(store.Path, []byte("private-corrupt-state"), 0600))
		}
		handler, err := auth.NewHandler("https://example.test", store)
		require.NoError(t, err)
		server := httptest.NewServer(handler)
		status, result := registrationRequest(t, server, http.MethodPost, `{"redirect_uris":["https://client.test/cb"]}`, "application/json")
		server.Close()
		require.Equal(t, http.StatusInternalServerError, status)
		require.Equal(t, map[string]any{"error": "server_error"}, result)
		body, err := os.ReadFile(store.Path)
		if corrupt {
			require.NoError(t, err)
			require.Equal(t, "private-corrupt-state", string(body))
		} else {
			require.ErrorIs(t, err, os.ErrNotExist)
		}
	}
}
