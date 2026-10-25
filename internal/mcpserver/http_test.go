package mcpserver_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
