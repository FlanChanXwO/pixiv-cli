package mcpserver_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	server "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	fanboxmcp "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox"
	pixivmcp "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestHTTPRoutesProtectMCPWithoutNormalizingPaths(t *testing.T) {
	for _, base := range []string{"https://example.test", "https://example.test/a%2Fb", "https://example.test//prefix"} {
		t.Run(base, func(t *testing.T) {
			store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
			_, err := store.Init(t.Context(), false)
			require.NoError(t, err)
			protocol := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			handler, err := server.NewHTTPHandler(t.Context(), protocol, base, store, nil)
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
	_, err := server.NewHTTPHandler(t.Context(), protocol, "https://example.test", store, nil)
	require.ErrorIs(t, err, auth.ErrNotInitialized)
	for _, base := range []string{"", "relative", "https://user:secret@example.test"} {
		_, err = server.NewHTTPHandler(t.Context(), protocol, base, store, nil)
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
	err = server.RunHTTP(t.Context(), protocol, listener.Addr().String(), "http://"+listener.Addr().String(), store, io.Discard, nil)
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
				err := server.RunHTTP(ctx, protocol, addr, "http://"+addr, store, writer, nil)
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
	handler, err := server.NewHTTPHandler(t.Context(), protocol, "https://example.test", store, nil)
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

// 只替换外部产品 HTTP 边界；OAuth 和 MCP 使用真实 loopback HTTP。
type oauthFixtureTransport func(*http.Request) (*http.Response, error)

func (f oauthFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPOAuthClientReadsBothProductsAfterRestart(t *testing.T) {
	ctx := t.Context()
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	owner, err := store.Init(ctx, false)
	require.NoError(t, err)
	var pixivReads, fanboxReads atomic.Int32
	productHTTP := &http.Client{Transport: oauthFixtureTransport(func(r *http.Request) (*http.Response, error) {
		body, mime := "", "application/json"
		switch {
		case r.URL.Host == "app-api.pixiv.net" && r.URL.Path == "/v1/user/detail":
			if r.Header.Get("Authorization") != "Bearer fixture-pixiv-access" || r.Header.Get("Cookie") != "" || r.URL.Query().Get("user_id") != "72" {
				return nil, errors.New("incorrect Pixiv account or request")
			}
			pixivReads.Add(1)
			body = `{"user":{"id":72,"name":"Pixiv fixture","account":"pixiv-fixture","profile_image_urls":{"medium":"https://example.test/avatar.png"}},"profile":{},"profile_publicity":{},"workspace":{}}`
		case r.URL.Host == "www.fanbox.cc" && r.URL.Path == "/":
			cookie, err := r.Cookie("FANBOXSESSID")
			if err != nil || cookie.Value != "fixture-fanbox-session" || r.Header.Get("Authorization") != "" {
				return nil, errors.New("incorrect FANBOX account or request")
			}
			fanboxReads.Add(1)
			mime = "text/html"
			body = `<html><head><meta name="metadata" content='{"context":{"user":{"userId":91,"name":"FANBOX fixture"}}}'></head></html>`
		default:
			return nil, errors.New("unexpected product request; real network disabled")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {mime}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	start := func(addr string) (*httptest.Server, context.CancelFunc) {
		lifetime, cancel := context.WithCancel(ctx)
		protocol := server.New(pixivmcp.SDKPorts{Execute: func(ctx context.Context, account pixivmcp.Account, call func(context.Context, *pixivsdk.Client) (bool, error)) error {
			if account.UserID != 72 {
				return errors.New("incorrect Pixiv selection")
			}
			client, err := pixivsdk.NewWith("fixture-pixiv-access", pixivsdk.Options{HTTPClient: productHTTP})
			if err != nil {
				return err
			}
			_, err = call(ctx, client)
			return err
		}}, pixivmcp.Account{UserID: 72}, fanboxmcp.SDKPorts{Open: func(context.Context, fanboxmcp.Account) (*fanboxsdk.Client, error) {
			return fanboxsdk.OpenWith(fanboxsdk.SessionCredentials{FANBOXSESSID: "fixture-fanbox-session"}, fanboxsdk.Options{HTTPClient: productHTTP})
		}}, nil)
		instance := httptest.NewUnstartedServer(nil)
		if addr != "" {
			require.NoError(t, instance.Listener.Close())
			instance.Listener, err = net.Listen("tcp", addr)
			require.NoError(t, err)
		}
		base := "http://" + instance.Listener.Addr().String() + "/remote"
		instance.Config.Handler, err = server.NewHTTPHandler(lifetime, protocol, base, auth.Store{Path: store.Path}, nil)
		require.NoError(t, err)
		instance.Start()
		t.Cleanup(func() { cancel(); instance.Close() })
		return instance, cancel
	}
	instance, cancel := start("")
	origin := instance.URL
	base := origin + "/remote"
	localTransport := http.DefaultTransport.(*http.Transport).Clone()
	localTransport.Proxy = nil
	t.Cleanup(localTransport.CloseIdleConnections)
	type event struct {
		Method, Path string
		Status       int
	}
	var mu sync.Mutex
	var events []event
	clientHTTP := &http.Client{Transport: oauthFixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme+"://"+r.URL.Host != origin {
			return nil, errors.New("unexpected OAuth destination")
		}
		response, err := localTransport.RoundTrip(r)
		if err == nil {
			mu.Lock()
			events = append(events, event{r.Method, r.URL.Path, response.StatusCode})
			mu.Unlock()
		}
		return response, err
	})}
	callbackResult := make(chan *mcpauth.AuthorizationResult, 1)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		callbackResult <- &mcpauth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state"), Iss: q.Get("iss")}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer callback.Close()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	browser := &http.Client{Jar: jar, Transport: oauthFixtureTransport(func(r *http.Request) (*http.Response, error) {
		destination := r.URL.Scheme + "://" + r.URL.Host
		if destination != origin && destination != callback.URL {
			return nil, errors.New("unexpected browser destination")
		}
		return localTransport.RoundTrip(r)
	})}
	fetcher := func(ctx context.Context, args *mcpauth.AuthorizationArgs) (*mcpauth.AuthorizationResult, error) {
		request, err := http.NewRequestWithContext(ctx, "GET", args.URL, nil)
		if err != nil {
			return nil, errors.New("invalid authorization URL")
		}
		response, err := browser.Do(request)
		if err != nil {
			return nil, errors.New("authorization page unavailable")
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 {
			return nil, errors.New("authorization page failed")
		}
		csrf := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindSubmatch(body)
		action := regexp.MustCompile(`action="([^"]+)"`).FindSubmatch(body)
		if len(csrf) != 2 || len(action) != 2 {
			return nil, errors.New("missing consent form")
		}
		form := url.Values{"csrf_token": {string(csrf[1])}, "owner_secret": {owner}, "decision": {"allow"}}
		request, err = http.NewRequestWithContext(ctx, "POST", html.UnescapeString(string(action[1])), strings.NewReader(form.Encode()))
		if err != nil {
			return nil, errors.New("invalid consent action")
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Origin", origin)
		response, err = browser.Do(request)
		if err != nil {
			return nil, errors.New("consent callback failed")
		}
		response.Body.Close()
		if response.StatusCode != 204 {
			return nil, errors.New("consent was not accepted")
		}
		select {
		case result := <-callbackResult:
			return result, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	var issuedConfig *oauth2.Config
	handler, err := mcpauth.NewAuthorizationCodeHandler(&mcpauth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &mcpauth.DynamicClientRegistrationConfig{Metadata: &oauthex.ClientRegistrationMetadata{
			ClientName: "Standard client fixture", RedirectURIs: []string{callback.URL}, TokenEndpointAuthMethod: "none", GrantTypes: []string{"authorization_code", "refresh_token"},
		}},
		AuthorizationCodeFetcher: fetcher, RequestRefreshToken: true, Client: clientHTTP,
		NewTokenSource: func(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (oauth2.TokenSource, error) {
			issuedConfig = cfg
			return cfg.TokenSource(ctx, token), nil
		},
	})
	require.NoError(t, err)
	connect := func(handler mcpauth.OAuthHandler) *mcp.ClientSession {
		client := mcp.NewClient(&mcp.Implementation{Name: "standard-oauth-fixture", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: base + "/mcp", HTTPClient: clientHTTP, OAuthHandler: handler}, nil)
		require.NoError(t, err)
		t.Cleanup(func() { session.Close() })
		return session
	}
	readProducts := func(session *mcp.ClientSession) {
		for _, tc := range []struct {
			name string
			args map[string]any
			want string
		}{
			{"pixiv_user_detail", map[string]any{"user_id": 72}, "Pixiv fixture"},
			{"fanbox_current_user", map[string]any{}, "FANBOX fixture"},
		} {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			require.NoError(t, err)
			require.False(t, result.IsError, tc.name)
			body, err := json.Marshal(result.StructuredContent)
			require.NoError(t, err)
			require.Contains(t, string(body), tc.want)
			require.NotContains(t, string(body), "fixture-pixiv-access")
			require.NotContains(t, string(body), "fixture-fanbox-session")
		}
	}
	session := connect(handler)
	tools, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Zero(t, pixivReads.Load())
	require.Zero(t, fanboxReads.Load())
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "pixiv_user_detail")
	require.Contains(t, names, "fanbox_current_user")
	require.NotContains(t, names, "user_detail")
	require.NotContains(t, names, "download")
	readProducts(session)
	source, err := handler.TokenSource(ctx)
	require.NoError(t, err)
	token, err := source.Token()
	require.NoError(t, err)
	require.NotEmpty(t, token.RefreshToken)
	state, err := store.Read(ctx)
	require.NoError(t, err)
	require.Len(t, state.Clients, 1)
	require.Len(t, state.Grants, 1)
	require.Contains(t, state.Clients, issuedConfig.ClientID)
	// 客户端持久化独立于服务端 hash state；仅使用私有临时文件。
	saved := struct {
		Config *oauth2.Config
		Token  *oauth2.Token
	}{issuedConfig, token}
	data, err := json.Marshal(saved)
	require.NoError(t, err)
	clientPath := filepath.Join(t.TempDir(), "client.json")
	require.NoError(t, os.WriteFile(clientPath, data, 0600))
	require.NoError(t, session.Close())
	addr := instance.Listener.Addr().String()
	cancel()
	instance.Close()
	instance, cancel = start(addr)
	defer cancel()
	reopened, err := (auth.Store{Path: store.Path}).Read(ctx)
	require.NoError(t, err)
	require.Equal(t, state, reopened)
	data, err = os.ReadFile(clientPath)
	require.NoError(t, err)
	saved.Config, saved.Token = nil, nil
	require.NoError(t, json.Unmarshal(data, &saved))
	// 让标准 token source 认为 access 已过期，无需等待一小时或改生产时钟。
	saved.Token.Expiry = time.Now().Add(-time.Minute)
	refreshCtx := context.WithValue(ctx, oauth2.HTTPClient, clientHTTP)
	_, err = saved.Config.TokenSource(refreshCtx, saved.Token).Token()
	var tokenError *oauth2.RetrieveError
	require.ErrorAs(t, err, &tokenError)
	require.Equal(t, "invalid_request", tokenError.ErrorCode)
	afterRejectedRefresh, err := store.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, reopened, afterRejectedRefresh)

	// go-sdk v1.8.0 的默认 oauth2.TokenSource 不带 resource；保留上面的
	// 不兼容证据。对照仅补 MCP 规范要求的 form 参数，不放宽服务端校验。
	resourceHTTP := &http.Client{Transport: oauthFixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.String() != saved.Config.Endpoint.TokenURL {
			return nil, errors.New("unexpected refresh destination")
		}
		body, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			return nil, err
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		form.Set("resource", base+"/mcp")
		request, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		request.Header = r.Header.Clone()
		return clientHTTP.Transport.RoundTrip(request)
	})}
	refreshCtx = context.WithValue(ctx, oauth2.HTTPClient, resourceHTTP)
	resumedSource := saved.Config.TokenSource(refreshCtx, saved.Token)
	resumed, err := mcpauth.NewAuthorizationCodeHandler(&mcpauth.AuthorizationCodeHandlerConfig{
		PreregisteredClient: &oauthex.ClientCredentials{ClientID: saved.Config.ClientID}, RedirectURL: callback.URL,
		AuthorizationCodeFetcher: func(context.Context, *mcpauth.AuthorizationArgs) (*mcpauth.AuthorizationResult, error) {
			return nil, errors.New("restart unexpectedly requires consent")
		},
		InitialTokenSource: resumedSource, Client: clientHTTP,
	})
	require.NoError(t, err)
	// 由 MCP transport 自动取得/刷新 token，不手工注入刷新后的 bearer。
	readProducts(connect(resumed))
	refreshed, err := resumedSource.Token()
	require.NoError(t, err)
	require.True(t, refreshed.RefreshToken != token.RefreshToken, "refresh token did not rotate")
	require.True(t, refreshed.AccessToken != token.AccessToken, "access token did not rotate")
	mu.Lock()
	trace := append([]event(nil), events...)
	mu.Unlock()
	for _, want := range []event{
		{"POST", "/remote/mcp", 401},
		{"GET", "/.well-known/oauth-protected-resource/remote/mcp", 200},
		{"GET", "/.well-known/oauth-authorization-server/remote", 200},
		{"POST", "/remote/oauth/register", 201},
		{"POST", "/remote/oauth/token", 200},
	} {
		require.Contains(t, trace, want)
	}
	afterRefresh, err := store.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, state.Clients, afterRefresh.Clients)
	require.Len(t, afterRefresh.Grants, 1)
	stored, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	for _, secret := range []string{owner, token.AccessToken, token.RefreshToken, refreshed.AccessToken, refreshed.RefreshToken} {
		if strings.Contains(string(stored), secret) {
			t.Fatal("raw OAuth credential persisted in server state")
		}
	}
	require.Equal(t, int32(2), pixivReads.Load())
	require.Equal(t, int32(2), fanboxReads.Load())
}

func TestHTTPMountsLoginRelayUnderCanonicalHostWithoutBearer(t *testing.T) {
	for _, base := range []string{"https://example.test", "https://example.test/a%2Fb", "https://example.test//prefix"} {
		t.Run(base, func(t *testing.T) {
			store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
			_, err := store.Init(t.Context(), false)
			require.NoError(t, err)
			login, err := accounts.NewLoginManager(t.Context(), base+"/pixiv-login", func() (accounts.LoginAttempt, error) {
				return accounts.LoginAttempt{AuthorizationURL: "https://app-api.pixiv.net/web/v1/login", AcceptsCallback: func(string) bool { return true }, Complete: func(context.Context, string) (accounts.LoginResult, error) { return accounts.LoginResult{}, nil }}, nil
			})
			require.NoError(t, err)
			defer login.Close()
			started, err := login.Start(false)
			require.NoError(t, err)
			protocol := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			handler, err := server.NewHTTPHandler(t.Context(), protocol, base, store, login)
			require.NoError(t, err)
			page := httptest.NewRecorder()
			handler.ServeHTTP(page, httptest.NewRequest("GET", started.AuthorizationURL, nil))
			require.Equal(t, http.StatusSeeOther, page.Code)
			link, err := url.Parse(page.Header().Get("Location"))
			require.NoError(t, err)
			startURL := link.Query().Get("origin") + "/start/" + link.Query().Get("session")
			wrong := httptest.NewRecorder()
			handler.ServeHTTP(wrong, httptest.NewRequest("POST", startURL, strings.NewReader(`{"proof":"wrong"}`)))
			require.Equal(t, http.StatusUnauthorized, wrong.Code)
			correct := httptest.NewRecorder()
			handler.ServeHTTP(correct, httptest.NewRequest("POST", startURL, strings.NewReader(`{"proof":"`+link.Query().Get("access")+`"}`)))
			require.Equal(t, http.StatusOK, correct.Code)
			request := httptest.NewRequest("GET", started.AuthorizationURL, nil)
			request.Host = "attacker.invalid"
			denied := httptest.NewRecorder()
			handler.ServeHTTP(denied, request)
			require.Equal(t, http.StatusForbidden, denied.Code)
		})
	}
}
