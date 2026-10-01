package mcpserver_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	server "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestHTTPRoutesProtectMCPWithoutNormalizingPaths(t *testing.T) {
	for _, base := range []string{"https://example.test", "https://example.test/a%2Fb", "https://example.test//prefix"} {
		t.Run(base, func(t *testing.T) {
			store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
			_, err := store.Init(t.Context(), false)
			require.NoError(t, err)
			protocol := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			handler, err := server.NewHTTPHandler(t.Context(), protocol, base, store)
			require.NoError(t, err)
			for _, method := range []string{"POST", "GET", "DELETE"} {
				request := httptest.NewRequest(method, base+"/mcp", nil)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				require.Equal(t, 401, response.Code)
				require.Contains(t, response.Header().Get("WWW-Authenticate"), "resource_metadata=")
				require.Equal(t, "no-store", response.Result().Header.Get("Cache-Control"))
			}
			parsed, err := url.Parse(base)
			require.NoError(t, err)
			for _, tc := range []struct {
				path, host string
				status     int
			}{
				{"/.well-known/oauth-authorization-server" + parsed.EscapedPath(), parsed.Host, 200},
				{parsed.EscapedPath() + "/mcp/", parsed.Host, 404},
				{parsed.EscapedPath() + "/mcp", "attacker.invalid", 403},
			} {
				response := httptest.NewRecorder()
				request := httptest.NewRequest("GET", "https://"+tc.host+tc.path, nil)
				handler.ServeHTTP(response, request)
				require.Equal(t, tc.status, response.Code, tc.path)
			}
		})
	}
}

func TestHTTPRequiresValidConfigurationAndOwner(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	protocol := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	_, err := server.NewHTTPHandler(t.Context(), protocol, "https://example.test", store)
	require.ErrorIs(t, err, auth.ErrNotInitialized)
	for _, base := range []string{"", "relative", "https://user:secret@example.test"} {
		_, err = server.NewHTTPHandler(t.Context(), protocol, base, store)
		require.Error(t, err)
		require.False(t, strings.Contains(err.Error(), "secret"))
	}
}

func TestHTTPListenerReportsBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err = store.Init(t.Context(), false)
	require.NoError(t, err)
	protocol := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	err = server.RunHTTP(t.Context(), protocol, listener.Addr().String(), "http://"+listener.Addr().String(), store, io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), listener.Addr().String())
}

func TestHTTPListenerServesAndStopsOnCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		disconnect    bool
	}{
		{"legacy_shutdown", "2025-11-25", false},
		{"modern_shutdown", "2026-07-28", false},
		{"modern_disconnect", "2026-07-28", true},
	} {
		t.Run(tc.name, func(t *testing.T) {

			reservation, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			addr := reservation.Addr().String()
			require.NoError(t, reservation.Close())
			store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
			_, err = store.Init(t.Context(), false)
			require.NoError(t, err)
			state, err := store.Read(t.Context())
			require.NoError(t, err)
			token := "fixture-lifecycle-access"
			sum := sha256.Sum256([]byte(token))
			state.Clients["fixture"] = auth.Client{RedirectURIs: []string{"https://client.test/callback"}}
			state.Grants["fixture"] = auth.Grant{ClientID: "fixture", Resource: "http://" + addr + "/mcp", Scope: "mcp", AccessTokens: map[string]time.Time{hex.EncodeToString(sum[:]): time.Now().Add(time.Hour)}, RefreshHash: strings.Repeat("a", 64)}
			body, err := json.Marshal(state)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(store.Path, body, 0600))
			protocol := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			started, stopped := make(chan struct{}), make(chan struct{})
			released := make(chan struct{})
			release := sync.OnceFunc(func() { close(released) })
			defer release()
			mcp.AddTool(protocol, &mcp.Tool{Name: "wait", Description: "Wait for cancellation"}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
				close(started)
				<-ctx.Done()
				close(stopped)
				<-released
				return nil, nil, ctx.Err()
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reader, writer := io.Pipe()
			defer reader.Close()
			finished := make(chan error, 1)
			go func() {
				err := server.RunHTTP(ctx, protocol, addr, "http://"+addr, store, writer)
				writer.CloseWithError(err)
				finished <- err
			}()
			line, err := bufio.NewReader(reader).ReadString(10)
			require.NoError(t, err)
			require.Contains(t, line, "http://"+addr+"/mcp")
			require.Contains(t, line, "initialized")
			request, err := http.NewRequestWithContext(t.Context(), "GET", "http://"+addr+"/mcp", nil)
			require.NoError(t, err)
			response, err := http.DefaultClient.Do(request)
			require.NoError(t, err)
			require.Equal(t, 401, response.StatusCode)
			response.Body.Close()
			params := map[string]any{"name": "wait", "arguments": map[string]any{}}
			if tc.version == "2026-07-28" {
				params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": tc.version, "io.modelcontextprotocol/clientInfo": map[string]any{"name": "fixture", "version": "1"}, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
			}
			body, err = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": params})
			require.NoError(t, err)
			requestCtx, cancelRequest := context.WithCancel(t.Context())
			defer cancelRequest()
			request, err = http.NewRequestWithContext(requestCtx, "POST", "http://"+addr+"/mcp", strings.NewReader(string(body)))
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			request.Header.Set("MCP-Protocol-Version", tc.version)
			request.Header.Set("Mcp-Method", "tools/call")
			request.Header.Set("Mcp-Name", "wait")
			requested := make(chan error, 1)
			go func() {
				response, err := http.DefaultClient.Do(request)
				if err == nil {
					_, err = io.Copy(io.Discard, response.Body)
					response.Body.Close()
				}
				requested <- err
			}()
			select {
			case <-started:
			case err := <-requested:
				t.Fatalf("request ended before tool started: %v", err)
			}
			if tc.disconnect {
				cancelRequest()
				select {
				case <-stopped:
				case <-time.After(5 * time.Second):
					t.Error("disconnected modern request did not cancel its tool")
				}
			}
			cancel()
			select {
			case <-stopped:
			case err := <-finished:
				t.Fatalf("server returned before canceling its active tool: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("server did not cancel its active tool")
			}
			select {
			case err := <-finished:
				t.Fatalf("server returned before tool cleanup completed: %v", err)
			default:
			}
			release()
			require.ErrorIs(t, <-finished, context.Canceled)
			<-requested
			rebound, err := net.Listen("tcp", addr)
			require.NoError(t, err)
			rebound.Close()

		})
	}
}

func TestHTTPAuthenticatedSDKNegotiationAndReset(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	state, err := store.Read(t.Context())
	require.NoError(t, err)
	token := "fixture-access-token"
	sum := sha256.Sum256([]byte(token))
	state.Clients["fixture"] = auth.Client{RedirectURIs: []string{"https://client.test/callback"}}
	state.Grants["fixture"] = auth.Grant{ClientID: "fixture", Resource: "https://example.test/mcp", Scope: "mcp", AccessTokens: map[string]time.Time{hex.EncodeToString(sum[:]): time.Now().Add(time.Hour)}, RefreshHash: strings.Repeat("a", 64)}
	body, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(store.Path, body, 0600))
	protocol := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	handler, err := server.NewHTTPHandler(t.Context(), protocol, "https://example.test", store)
	require.NoError(t, err)
	for _, version := range []string{"2025-11-25", "2026-07-28"} {
		for _, origin := range []string{"https://example.test", "https://EXAMPLE.test:443"} {
			method := "initialize"
			params := map[string]any{"protocolVersion": version, "clientInfo": map[string]any{"name": "fixture", "version": "1"}, "capabilities": map[string]any{}}
			if version == "2026-07-28" {
				method = "server/discover"
				params = map[string]any{"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": version, "io.modelcontextprotocol/clientInfo": map[string]any{"name": "fixture", "version": "1"}, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}}
			}
			body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
			require.NoError(t, err)
			request := httptest.NewRequest("POST", "https://example.test/mcp", strings.NewReader(string(body)))
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			request.Header.Set("MCP-Protocol-Version", version)
			request.Header.Set("Mcp-Method", method)
			request.Header.Set("Origin", origin)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, 200, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"result"`)
			require.NotContains(t, response.Body.String(), `"error"`)
			require.Empty(t, response.Header().Get("Mcp-Session-Id"))
			require.Equal(t, "no-store", response.Result().Header.Get("Cache-Control"))
		}
	}
	_, err = store.Init(t.Context(), true)
	require.NoError(t, err)
	request := httptest.NewRequest("GET", "https://example.test/mcp", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, 401, response.Code)
}
