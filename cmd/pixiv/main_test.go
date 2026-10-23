package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

const versionSmokeConfig = "[update]\ncheck_enabled = false\n"

func TestPixivBinaryReportsRootVersion(t *testing.T) {
	t.Run("development build uses defaults", func(t *testing.T) {
		assertPixivVersionContract(t, buildPixivBinary(t), "dev")
	})

	t.Run("linker injected build uses release version", func(t *testing.T) {
		binaryPath := buildPixivBinary(t, "-X github.com/FlanChanXwO/pixiv-cli/internal/version.Version=v0.1.0")
		assertPixivVersionContract(t, binaryPath, "v0.1.0")
	})
}

// TestPixivBinaryPackagedSmoke 验证平台 workflow 解包后的实际二进制，而不是重新构建一个开发二进制。
// 该测试只在 workflow 注入路径与期望版本时运行；不读取凭据，也不启用真实 SDK E2E。
func TestPixivBinaryPackagedSmoke(t *testing.T) {
	binaryPath := strings.TrimSpace(os.Getenv("PIXIV_E2E_BINARY"))
	if binaryPath == "" {
		t.Skip("PIXIV_E2E_BINARY is not set")
	}
	if !filepath.IsAbs(binaryPath) {
		t.Fatal("PIXIV_E2E_BINARY must be an absolute path")
	}
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("stat packaged binary: %v", err)
	}
	expectedVersion := strings.TrimSpace(os.Getenv("PIXIV_E2E_EXPECTED_VERSION"))
	if expectedVersion == "" {
		t.Fatal("PIXIV_E2E_EXPECTED_VERSION is required with PIXIV_E2E_BINARY")
	}

	assertPixivVersionContract(t, binaryPath, expectedVersion)
	for _, args := range [][]string{{"config", "path"}, {"mcp", "--help"}, {"fanbox", "--help"}} {
		t.Run(fmt.Sprintf("%s %s", args[0], strings.Join(args[1:], " ")), func(t *testing.T) {
			stdout := runPixivCommand(t, binaryPath, args...)
			if strings.TrimSpace(stdout) == "" {
				t.Fatalf("pixiv %s returned empty stdout", strings.Join(args, " "))
			}
		})
	}
	t.Run("removed FANBOX MCP command", func(t *testing.T) {
		stdout, stderr, err := runPixivProcess(t, binaryPath, "fanbox", "mcp")
		if err == nil || stdout != "" || !strings.Contains(stderr, "usage: pixiv fanbox <command>") {
			t.Fatalf("removed FANBOX MCP command: err=%v stdout=%q stderr=%q", err, stdout, stderr)
		}
	})
}

// TestPixivBinaryMCPCancelsOnSignals 验证真实入口把进程信号传入已有 HTTP 关闭路径；
// 在途工具的取消与清理顺序由 mcpserver 的 HTTP lifecycle 测试覆盖。
func TestPixivBinaryMCPCancelsOnSignals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal cannot deliver interrupt/termination signals on Windows")
	}
	binaryPath := packagedOrBuiltBinary(t)
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			env := isolatedEnv(t)
			prepareMCPHome(t, env, binaryPath)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			init := exec.CommandContext(ctx, binaryPath, "mcp", "auth", "init")
			init.Dir, init.Env = repoRoot, env.values
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
			command.Dir, command.Env = repoRoot, env.values
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
	binary := packagedOrBuiltBinary(t)
	env := isolatedEnv(t)
	prepareMCPHome(t, env, binary)
	command := exec.CommandContext(t.Context(), binary, "mcp")
	command.Dir, command.Env = repoRoot, env.values
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
	_, err = os.Stat(filepath.Join(env.home, ".pixiv-cli", "mcp-state.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Empty(t, diagnostics.String())
}

func TestPixivBinaryHTTPOAuthDiscovery(t *testing.T) {
	binary := packagedOrBuiltBinary(t)
	env := isolatedEnv(t)
	prepareMCPHome(t, env, binary)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	init := exec.CommandContext(ctx, binary, "mcp", "auth", "init")
	init.Dir, init.Env = repoRoot, env.values
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
	command.Dir, command.Env = repoRoot, env.values
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
	before, err := os.ReadFile(filepath.Join(env.home, ".pixiv-cli", "mcp-state.json"))
	require.NoError(t, err)
	localCommand := exec.CommandContext(ctx, binary, "mcp")
	localCommand.Dir, localCommand.Env = repoRoot, env.values
	local, err := mcp.NewClient(&mcp.Implementation{Name: "concurrent-local", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: localCommand}, nil)
	require.NoError(t, err)
	localResult, err := local.CallTool(ctx, &mcp.CallToolParams{Name: "pixiv_illust_detail", Arguments: map[string]any{"illust_id": 1}})
	require.NoError(t, err)
	require.True(t, localResult.IsError)
	require.NoError(t, local.Close())
	after, err := os.ReadFile(filepath.Join(env.home, ".pixiv-cli", "mcp-state.json"))
	require.NoError(t, err)
	require.True(t, bytes.Equal(before, after), "concurrent stdio must preserve HTTP owner/grant state")

	tools, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	names := []string{}
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	require.Len(t, names, 54)
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

const repoRoot = "../.."

func assertPixivVersionContract(t *testing.T, binaryPath, version string) {
	t.Helper()
	stdout := runPixivCommand(t, binaryPath, "--version")
	if want := "pixiv " + version + "\n"; stdout != want {
		t.Fatalf("pixiv --version stdout = %q, want %q", stdout, want)
	}
}

func buildPixivBinary(t *testing.T, ldflags ...string) string {
	t.Helper()
	binaryName := "pixiv"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	args := []string{"build", "-trimpath", "-o", binaryPath}
	if len(ldflags) > 0 {
		args = append(args, "-ldflags", strings.Join(ldflags, " "))
	}
	args = append(args, "./cmd/pixiv")

	build := exec.CommandContext(t.Context(), "go", args...)
	build.Dir = repoRoot
	var stderr bytes.Buffer
	build.Stderr = &stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return binaryPath
}

func packagedOrBuiltBinary(t *testing.T) string {
	t.Helper()
	binaryPath := strings.TrimSpace(os.Getenv("PIXIV_E2E_BINARY"))
	if binaryPath == "" {
		return buildPixivBinary(t)
	}
	require.True(t, filepath.IsAbs(binaryPath), "binary path must be absolute")
	resolved, err := filepath.EvalSymlinks(binaryPath)
	require.NoError(t, err)
	return resolved
}

func runPixivCommand(t *testing.T, binaryPath string, args ...string) string {
	t.Helper()
	stdout, stderr, err := runPixivProcess(t, binaryPath, args...)
	if err != nil {
		t.Fatalf("pixiv %s exited unsuccessfully: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("pixiv %s wrote to stderr:\n%s", strings.Join(args, " "), stderr)
	}
	return stdout
}

func runPixivProcess(t *testing.T, binaryPath string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	run := exec.CommandContext(t.Context(), binaryPath, args...)
	run.Dir = repoRoot
	run.Env = isolatedEnv(t).values
	var stdoutBuffer, stderrBuffer bytes.Buffer
	run.Stdout = &stdoutBuffer
	run.Stderr = &stderrBuffer
	err = run.Run()
	return stdoutBuffer.String(), stderrBuffer.String(), err
}

// isolatedProcessEnv 为子进程提供与宿主用户目录和 CLI 覆写隔离的环境。
type isolatedProcessEnv struct {
	values []string
	home   string
}

// isolatedEnv 同时关闭临时用户目录中的自动更新，避免网络提示污染命令 stderr。
func isolatedEnv(t *testing.T) isolatedProcessEnv {
	var env isolatedProcessEnv
	t.Helper()
	env.home = t.TempDir()
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if found && isIsolatedEnvKey(name) {
			continue
		}
		env.values = append(env.values, entry)
	}
	env.values = append(env.values,
		"HOME="+env.home,
		"APPDATA="+env.home,
		"LOCALAPPDATA="+env.home,
		"USERPROFILE="+env.home,
	)
	configDirectory := filepath.Join(env.home, ".pixiv-cli")
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		t.Fatalf("create isolated config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDirectory, "config.toml"), []byte(versionSmokeConfig), 0o600); err != nil {
		t.Fatalf("write isolated config: %v", err)
	}
	return env
}

func prepareMCPHome(t *testing.T, env isolatedProcessEnv, binaryPath string) {
	t.Helper()
	appDir := filepath.Join(env.home, ".pixiv-cli")
	require.NoError(t, os.MkdirAll(filepath.Join(appDir, "url-handler"), 0o700))
	// 临时 manifest 只避免启动钩子修改真实 OS 协议关联，不代表 helper 已安装。
	manifest, err := json.Marshal(map[string]any{"version": 1, "executable_path": binaryPath, "home_directory": env.home})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "url-handler", "handler-manifest.json"), manifest, 0o600))
}

func isIsolatedEnvKey(name string) bool {
	for _, key := range []string{
		"HOME", "XDG_CONFIG_HOME", "APPDATA", "LOCALAPPDATA", "USERPROFILE", "HOMEDRIVE", "HOMEPATH",
		"DOWNLOAD_PATH", "FILENAME_TEMPLATE", "https_proxy", "HTTPS_PROXY",
	} {
		if strings.EqualFold(name, key) {
			return true
		}
	}
	return false
}
