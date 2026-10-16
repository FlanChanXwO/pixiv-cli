package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// TestPixivBinaryMCPCancelsOnSignals 验证真实入口把进程信号传入已有 HTTP 关闭路径；
// 在途工具的取消与清理顺序由 mcpserver 的 HTTP lifecycle 测试覆盖。
func TestPixivBinaryMCPCancelsOnSignals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal cannot deliver interrupt/termination signals on Windows")
	}
	binaryPath := strings.TrimSpace(os.Getenv("PIXIV_E2E_BINARY"))
	if binaryPath == "" {
		binaryPath = buildPixivVersionBinary(t, "..")
	}
	require.True(t, filepath.IsAbs(binaryPath), "binary path must be absolute")
	binaryPath, err := filepath.EvalSymlinks(binaryPath)
	require.NoError(t, err)
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			env := isolatedEnv(t)
			appDir := filepath.Join(env.home, ".pixiv-cli")
			require.NoError(t, os.MkdirAll(filepath.Join(appDir, "url-handler"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(appDir, "config.toml"), []byte(versionSmokeConfig), 0o600))
			// 临时 manifest 只避免启动钩子修改真实 OS 协议关联，不代表 helper 已安装。
			manifest, err := json.Marshal(map[string]any{"version": 1, "executable_path": binaryPath, "home_directory": env.home})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(appDir, "url-handler", "handler-manifest.json"), manifest, 0o600))

			ctx, cancel := context.WithCancel(testCommandContext(t))
			defer cancel()
			init := exec.CommandContext(ctx, binaryPath, "mcp", "auth", "init")
			init.Dir, init.Env = "..", env.values
			// owner secret 的一次性输出丢弃；本测试只需要服务启动，不进行授权。
			require.NoError(t, init.Run())
			reservation, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			addr := reservation.Addr().String()
			require.NoError(t, reservation.Close())
			endpoint := "http://" + addr + "/mcp"
			_, port, err := net.SplitHostPort(addr)
			require.NoError(t, err)
			command := exec.CommandContext(ctx, binaryPath, "mcp", "--port", port)
			command.Dir, command.Env = "..", env.values
			var stdout bytes.Buffer
			command.Stdout = &stdout
			stderr, err := command.StderrPipe()
			require.NoError(t, err)
			require.NoError(t, command.Start())
			waited := false
			defer func() {
				cancel()
				if !waited {
					_ = command.Wait()
				}
			}()
			reader := bufio.NewReader(stderr)
			line, err := reader.ReadString('\n')
			require.NoError(t, err)
			require.Contains(t, line, "MCP owner initialized; endpoint: "+endpoint)
			transport := &http.Transport{}
			defer transport.CloseIdleConnections()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			require.NoError(t, err)
			response, err := (&http.Client{Transport: transport}).Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusUnauthorized, response.StatusCode)

			require.NoError(t, command.Process.Signal(signal))
			tail, err := io.ReadAll(reader)
			require.NoError(t, err)
			err = command.Wait()
			waited = true
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit, "signal must report CLI cancellation")
			require.Equal(t, 1, exit.ExitCode(), "signal must reach CLI cleanup, not terminate the process directly")
			require.Contains(t, string(tail), "context canceled")
			require.Empty(t, stdout.String(), "MCP must not write JSON-RPC or secrets to stdout")
		})
	}
}

func TestPixivBinaryUnifiedStdioWithoutOwner(t *testing.T) {
	binary := strings.TrimSpace(os.Getenv("PIXIV_E2E_BINARY"))
	if binary == "" {
		binary = buildPixivVersionBinary(t, "..")
	}
	env := isolatedEnv(t)
	appDir := filepath.Join(env.home, ".pixiv-cli")
	require.NoError(t, os.MkdirAll(filepath.Join(appDir, "url-handler"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "config.toml"), []byte(versionSmokeConfig), 0600))
	manifest, err := json.Marshal(map[string]any{"version": 1, "executable_path": binary, "home_directory": env.home})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "url-handler", "handler-manifest.json"), manifest, 0600))
	command := exec.CommandContext(testCommandContext(t), binary, "mcp")
	command.Dir, command.Env = "..", env.values
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics
	session, err := mcp.NewClient(&mcp.Implementation{Name: "binary-stdio", Version: "1"}, nil).Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	require.NoError(t, err)
	defer session.Close()
	tools, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	require.Len(t, names, 49)
	require.Contains(t, names, "pixiv_comments")
	require.Contains(t, names, "fanbox_feed")
	require.NotContains(t, names, "pixiv_illust_comments")
	require.Contains(t, names, "pixiv_search_illust")
	require.Contains(t, names, "fanbox_post")
	require.Contains(t, names, "pixiv_download")
	require.NotContains(t, names, "pixiv_show_gallery")
	invalid, callErr := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "pixiv_comments", Arguments: map[string]any{"kind": "novel", "id": 0}})
	require.NoError(t, callErr)
	require.True(t, invalid.IsError)

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "pixiv_download", Arguments: map[string]any{"pages": "0", "src": "42"}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.NoError(t, session.Close())
	_, err = os.Stat(filepath.Join(appDir, "mcp-state.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Empty(t, diagnostics.String())
}

func TestPixivBinaryHTTPOAuthDiscovery(t *testing.T) {
	binary := strings.TrimSpace(os.Getenv("PIXIV_E2E_BINARY"))
	if binary == "" {
		binary = buildPixivVersionBinary(t, "..")
	}
	env := isolatedEnv(t)
	appDir := filepath.Join(env.home, ".pixiv-cli")
	require.NoError(t, os.MkdirAll(filepath.Join(appDir, "url-handler"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "config.toml"), []byte(versionSmokeConfig), 0600))
	manifest, err := json.Marshal(map[string]any{"version": 1, "executable_path": binary, "home_directory": env.home})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "url-handler", "handler-manifest.json"), manifest, 0600))
	ctx, cancel := context.WithCancel(testCommandContext(t))
	defer cancel()
	init := exec.CommandContext(ctx, binary, "mcp", "auth", "init")
	init.Dir, init.Env = "..", env.values
	secret, err := init.Output()
	require.NoError(t, err)
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := reservation.Addr().String()
	require.NoError(t, reservation.Close())
	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	base := "http://" + addr
	command := exec.CommandContext(ctx, binary, "mcp", "--port", port)
	command.Dir, command.Env = "..", env.values
	stderr, err := command.StderrPipe()
	require.NoError(t, err)
	require.NoError(t, command.Start())
	defer func() { cancel(); _ = command.Wait() }()
	readiness := bufio.NewReader(stderr)
	line, err := readiness.ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, line, base+"/mcp")
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	browser := &http.Client{Transport: transport, Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := browser.Post(base+"/oauth/register", "application/json", strings.NewReader(`{"redirect_uris":["http://127.0.0.1/callback"]}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var client struct {
		ID string `json:"client_id"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&client))
	require.NoError(t, response.Body.Close())
	query := url.Values{"client_id": {client.ID}, "redirect_uri": {"http://127.0.0.1/callback"}, "response_type": {"code"}, "resource": {base + "/mcp"}, "scope": {"mcp"}, "code_challenge_method": {"S256"}, "code_challenge": {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}}
	response, err = browser.Get(base + "/oauth/authorize?" + query.Encode())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	csrf := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindSubmatch(body)
	require.Len(t, csrf, 2)
	consent := url.Values{"csrf_token": {string(csrf[1])}, "owner_secret": {strings.TrimSpace(string(secret))}, "decision": {"allow"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/oauth/authorize", strings.NewReader(consent.Encode()))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", base)
	response, err = browser.Do(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, response.StatusCode)
	location, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	tokenForm := url.Values{"grant_type": {"authorization_code"}, "code": {location.Query().Get("code")}, "client_id": {client.ID}, "redirect_uri": {"http://127.0.0.1/callback"}, "resource": {base + "/mcp"}, "code_verifier": {"dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"}}
	response, err = browser.PostForm(base+"/oauth/token", tokenForm)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var token struct {
		Access string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&token))
	require.NoError(t, response.Body.Close())
	httpClient := &http.Client{Transport: mcpBearerTransport{transport, token.Access}}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "binary-http", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: base + "/mcp", HTTPClient: httpClient, DisableStandaloneSSE: true}, nil)
	require.NoError(t, err)
	defer session.Close()
	before, err := os.ReadFile(filepath.Join(appDir, "mcp-state.json"))
	require.NoError(t, err)
	localCommand := exec.CommandContext(ctx, binary, "mcp")
	localCommand.Dir, localCommand.Env = "..", env.values
	local, err := mcp.NewClient(&mcp.Implementation{Name: "concurrent-local", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: localCommand}, nil)
	require.NoError(t, err)
	localResult, err := local.CallTool(ctx, &mcp.CallToolParams{Name: "pixiv_illust_detail", Arguments: map[string]any{"illust_id": 1}})
	require.NoError(t, err)
	require.True(t, localResult.IsError)
	require.NoError(t, local.Close())
	after, err := os.ReadFile(filepath.Join(appDir, "mcp-state.json"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(before, after), "concurrent stdio must preserve HTTP owner/grant state")

	tools, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	require.Len(t, names, 53)
	require.Contains(t, names, "pixiv_comments")
	require.Contains(t, names, "fanbox_feed")
	require.NotContains(t, names, "fanbox_home")
	require.Contains(t, names, "pixiv_show_gallery")
	require.Contains(t, names, "fanbox_current_user")
	require.NotContains(t, names, "pixiv_download")
	invalid, callErr := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "pixiv_comments", Arguments: map[string]any{"kind": "novel", "id": 0}})
	require.NoError(t, callErr)
	require.True(t, invalid.IsError)

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pixiv_illust_detail", Arguments: map[string]any{"illust_id": 1}})
	require.NoError(t, err)
	require.True(t, result.IsError, "empty local accounts must fail structurally without upstream access")
}

type mcpBearerTransport struct {
	next  http.RoundTripper
	token string
}

func (t mcpBearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", "Bearer "+t.token)
	return t.next.RoundTrip(request)
}
