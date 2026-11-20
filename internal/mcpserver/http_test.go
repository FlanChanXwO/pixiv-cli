package mcpserver_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/loginrelay"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/stretchr/testify/require"
)

func TestHTTPRequiresBearer(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	state, err := store.Read(t.Context())
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("valid-bearer"))
	hash := hex.EncodeToString(sum[:])
	state.Clients["client"] = auth.Client{RedirectURIs: []string{"https://client.test/callback"}}
	state.Grants["grant"] = auth.Grant{ClientID: "client", Resource: "https://mcp.test/mcp", Scope: "mcp", RefreshHash: hash, AccessTokens: map[string]time.Time{hash: time.Now().Add(time.Hour)}}
	body, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.Path, body, 0o600))
	server := mcpserver.New(pixiv.SDKPorts{}, pixiv.Account{}, fanbox.SDKPorts{}, nil)
	handler, err := mcpserver.NewHTTPHandler(t.Context(), server, "https://mcp.test", store, nil, nil)
	require.NoError(t, err)
	for _, tc := range []struct {
		token  string
		status int
	}{{"", http.StatusUnauthorized}, {"unknown", http.StatusUnauthorized}, {"valid-bearer", http.StatusOK}} {
		request := httptest.NewRequest(http.MethodPost, "https://mcp.test/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		if tc.token != "" {
			request.Header.Set("Authorization", "Bearer "+tc.token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, tc.status, response.Code)
	}
}

func TestConsentPageLanguages(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	state, err := store.Read(t.Context())
	require.NoError(t, err)
	state.Clients["client"] = auth.Client{ClientName: "<script>client</script>", RedirectURIs: []string{"https://client.test/callback"}}
	body, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.Path, body, 0o600))
	handler, err := auth.NewHandler("https://mcp.test", store)
	require.NoError(t, err)
	for _, tc := range []struct{ header, locale, heading string }{
		{"en;q=0.4, zh-TW;q=0.9", "zh-CN", "授权 pixiv-cli"},
		{"zh;q=0, en;q=1", "en", "Authorize pixiv-cli"},
	} {
		t.Run(tc.locale, func(t *testing.T) {
			t.Parallel()
			query := url.Values{"client_id": {"client"}, "redirect_uri": {"https://client.test/callback"}, "response_type": {"code"}, "resource": {"https://mcp.test/mcp"}, "scope": {"mcp"}, "code_challenge_method": {"S256"}, "code_challenge": {strings.Repeat("A", 43)}}
			request := httptest.NewRequest(http.MethodGet, "https://mcp.test/oauth/authorize?"+query.Encode(), nil)
			request.Header.Set("Accept-Language", tc.header)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, tc.locale, response.Header().Get("Content-Language"))
			require.Contains(t, response.Body.String(), `<html lang="`+tc.locale+`">`)
			require.Contains(t, response.Body.String(), tc.heading)
			require.Contains(t, response.Body.String(), "&lt;script&gt;client&lt;/script&gt;")
			require.NotContains(t, response.Body.String(), "<script>client</script>")
		})
	}
}

func TestLoginPageLanguages(t *testing.T) {
	for _, locale := range []i18n.Locale{i18n.English, i18n.Chinese} {
		t.Run(locale.String(), func(t *testing.T) {
			t.Parallel()
			var manual bytes.Buffer
			require.NoError(t, loginrelay.WriteManual(&manual, "https://accounts.pixiv.net/?x=1&y=2", locale))
			require.Contains(t, manual.String(), `lang="`+locale.String()+`"`)
			require.Contains(t, manual.String(), locale.Text("login.finish"))
			require.Contains(t, manual.String(), `x=1&amp;y=2`)
			var callback bytes.Buffer
			require.NoError(t, loginrelay.WriteCallbackRelay(&callback, locale))
			require.Contains(t, callback.String(), locale.Text("login.completing"))
			response := httptest.NewRecorder()
			loginrelay.WriteFinalPage(response, false, locale)
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Equal(t, locale.String(), response.Header().Get("Content-Language"))
			require.Contains(t, response.Body.String(), locale.Text("login.failure_heading"))
		})
	}
}
