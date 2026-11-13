package mcpserver_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestToolsListContract(t *testing.T) {
	store := auth.Store{Path: filepath.Join(t.TempDir(), "state.json")}
	_, err := store.Init(t.Context(), false)
	require.NoError(t, err)
	media, err := mediaproxy.New("https://mcp.test", store, nil)
	require.NoError(t, err)
	login, err := accounts.NewLogin(t.Context(), "https://mcp.test/pixiv-login", func() (accounts.LoginAttempt, error) { panic("discovery must not start login") })
	require.NoError(t, err)
	defer login.Close()
	ports := pixiv.SDKPorts{Media: media, Downloads: &pixiv.LocalDownloads{}, Login: login, Accounts: accounts.Selection{Store: store, Load: func(context.Context) (accounts.LocalSnapshot, error) { panic("discovery must not read accounts") }}}
	server := mcpserver.New(ports, pixiv.Account{}, tools.SDKPorts{}, nil)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, serverSession.Close()) })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "g44-snapshot", Version: "1"}, nil).Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	var tools []*mcp.Tool
	for tool, err := range session.Tools(t.Context(), nil) {
		require.NoError(t, err)
		tools = append(tools, tool)
	}
	actual, err := json.MarshalIndent(tools, "", "  ")
	require.NoError(t, err)
	want, err := os.ReadFile(filepath.Join("testdata", "tools-list.golden.json"))
	require.NoError(t, err)
	require.Equal(t, string(want), string(actual)+"\n")
}

func TestToolOutputKeepsSignedURLAndCookiePrivate(t *testing.T) {
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "FANBOXSESSID=session-canary" {
			t.Error("request did not carry the session canary")
		}
		fmt.Fprint(w, `{"body":{"post":{"id":"p1","title":"private post","publishedDatetime":"2024-06-01T10:00:00Z","creatorId":"artist","body":{"images":[{"id":"img1","extension":"png","originalUrl":"https://downloads.fanbox.cc/image.png?signature=media-canary"}]}}}}`)
	}))
	defer api.Close()
	httpClient := api.Client()
	transport := httpClient.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
	}
	client, err := fanbox.New("session-canary", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	server := mcpserver.New(pixiv.SDKPorts{}, pixiv.Account{}, tools.SDKPorts{OpenLease: func(context.Context, tools.Account) (*lifecycle.Lease[*fanbox.Client], error) {
		return lifecycle.NewLease(client, func() error { client.CloseIdleConnections(); return nil }), nil
	}}, nil)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, serverSession.Close()) })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "privacy", Version: "1"}, nil).Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "fanbox_post", Arguments: map[string]any{"post_id": "p1"}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(body), "requires_credentials")
	for _, secret := range []string{"session-canary", "media-canary", "https://downloads.fanbox.cc/image.png"} {
		require.NotContains(t, string(body), secret)
	}
}

func TestInvalidToolInputKeepsCredentialsPrivate(t *testing.T) {
	server := mcpserver.New(pixiv.SDKPorts{}, pixiv.Account{}, tools.SDKPorts{}, nil)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "input-privacy", Version: "1"}, nil).Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()
	for _, input := range []struct {
		tool, field string
		structured  bool
	}{
		{"fanbox_creator", "creator_id", false},
		{"pixiv_user_detail", "user_id", false},
		{"fanbox_open_resource", "ref", true},
	} {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: input.tool, Arguments: map[string]any{input.field: map[string]any{"Cookie": "cookie-canary", "url": "https://media.test/a?signature=signature-canary"}}})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Equal(t, input.structured, result.StructuredContent != nil)
		wire, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(wire), "cookie-canary")
		require.NotContains(t, string(wire), "signature-canary")
		require.Contains(t, string(wire), "invalid_request")
	}
}

func TestSDKCursorKeepsSelectedAccount(t *testing.T) {
	var selected []int64
	api := httptest.NewTLSServer(http.HandlerFunc(cursorArtworkPage))
	defer api.Close()
	httpClient := api.Client()
	httpClient.Transport.(*http.Transport).DialTLSContext = func(context.Context, string, string) (net.Conn, error) {
		return tls.Dial("tcp", api.Listener.Addr().String(), httpClient.Transport.(*http.Transport).TLSClientConfig)
	}
	ports := pixiv.SDKPorts{ExecuteSDK: func(ctx context.Context, account pixiv.Account, attempt func(context.Context, int64, *pixivsdk.Client) (bool, error)) error {
		if account.UserID == 0 {
			account.UserID = int64(42 + len(selected)*42)
		}
		selected = append(selected, account.UserID)
		store, err := pixivsdk.NewMemoryCredentials(pixivsdk.Credentials{UserID: account.UserID, Revision: 1, AccessToken: credential.New([]byte("fixture-access"))})
		require.NoError(t, err)
		client, err := pixivsdk.New(store, pixivsdk.Options{HTTPClient: httpClient})
		require.NoError(t, err)
		_, err = attempt(ctx, account.UserID, client)
		return err
	}}
	server := mcpserver.New(ports, pixiv.Account{}, tools.SDKPorts{}, nil)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "cursor", Version: "1"}, nil).Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()
	args := map[string]any{}
	for range 2 {
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "pixiv_illust_ranking", Arguments: args})
		require.NoError(t, err)
		require.False(t, result.IsError, "%s", result.Content[0].(*mcp.TextContent).Text)
		args["cursor"] = result.StructuredContent.(map[string]any)["pagination"].(map[string]any)["next_cursor"]
	}
	require.Equal(t, []int64{42, 42}, selected)
}

func cursorArtworkPage(w http.ResponseWriter, r *http.Request) {
	next := "null"
	if !r.URL.Query().Has("offset") {
		next = `"https://app-api.pixiv.net/v1/illust/ranking?mode=day&offset=1"`
	}
	fmt.Fprintf(w, `{"illusts":[{"id":101,"title":"artwork","type":"illust","create_date":"2026-01-01T00:00:00Z","user":{"id":42,"name":"artist"},"width":640,"height":480,"meta_single_page":{"original_image_url":"https://i.pximg.net/a.jpg"},"page_count":1}],"next_url":%s}`, next)
}
