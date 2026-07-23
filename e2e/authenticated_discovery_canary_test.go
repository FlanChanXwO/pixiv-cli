package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// requireAuthenticatedDiscoveryCanary 只在调用者明确允许真实 App API 后启用。
// 它与现有认证 canary 共享凭据隔离策略，且整个 suite 不执行任何收藏或关注写操作。
func requireAuthenticatedDiscoveryCanary(t *testing.T) authenticatedCanaryAuth {
	t.Helper()
	if os.Getenv("PIXIV_E2E_REAL_API") != "1" {
		t.Skip("set PIXIV_E2E_REAL_API=1 to run authenticated discovery canary")
	}
	auth, skip, err := resolveAuthenticatedCanaryAuth(os.Getenv("PIXIV_E2E_REFRESH_TOKEN"), os.Getenv("PIXIV_E2E_USE_LOCAL_AUTH"))
	if err != nil {
		t.Fatalf("invalid authenticated discovery canary credential mode: %v", err)
	}
	if skip {
		t.Skip("set PIXIV_E2E_REFRESH_TOKEN or PIXIV_E2E_USE_LOCAL_AUTH=1 with PIXIV_E2E_REAL_API=1 to run authenticated discovery canary")
	}
	return auth
}

// TestPixivSDKAuthenticatedDiscoveryCanary 验证小说、官方作者搜索和指定全年龄
// 插画作者的 public SDK 路径。搜索词与作品 ID 均由调用者提供，不固化真实目标。
func TestPixivSDKAuthenticatedDiscoveryCanary(t *testing.T) {
	auth := requireAuthenticatedDiscoveryCanary(t)
	if err := validateAuthenticatedSDKCanaryBinary(os.Getenv("PIXIV_E2E_BINARY")); err != nil {
		t.Fatal(err)
	}
	words, err := authenticatedCanarySearchWordsFromEnvironment(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	sfwIllustID, err := authenticatedCanarySFWIllustIDFromEnvironment(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	proxy := firstNonEmpty(os.Getenv("PIXIV_E2E_PROXY"), os.Getenv("PIXIV_WEB_API_PROXY"))
	options, err := authenticatedCanarySDKOptions(t, auth, proxy)
	if err != nil {
		t.Fatalf("prepare authenticated discovery SDK canary: %v", err)
	}
	client, err := openAuthenticatedCanarySnapshot(testCommandContext(t), options)
	if err != nil {
		t.Fatalf("open authenticated discovery SDK snapshot: %v", err)
	}

	novels, err := client.SearchNovel(testCommandContext(t), pixiv.SearchNovelRequest{Word: words.discovery})
	if err != nil {
		t.Fatalf("authenticated novel search failed: %v", err)
	}
	if len(novels.Novels) == 0 {
		t.Fatalf("authenticated novel search %q returned no results", words.discovery)
	}
	if novels.Novels[0].ID <= 0 || novels.Novels[0].URL == "" {
		t.Fatalf("authenticated novel search returned incomplete first result: %+v", novels.Novels[0])
	}

	users, err := client.SearchUser(testCommandContext(t), pixiv.SearchUserRequest{Word: words.discovery})
	if err != nil {
		t.Fatalf("authenticated user search failed: %v", err)
	}
	if users.Source != pixiv.UserSearchSourceApp {
		t.Fatalf("authenticated user search source=%q, want %q", users.Source, pixiv.UserSearchSourceApp)
	}
	if len(users.UserPreviews) == 0 || users.UserPreviews[0].User.ID <= 0 {
		t.Fatalf("authenticated user search %q returned no usable results", words.discovery)
	}
	requireAuthenticatedCanaryAuthorSDK(t, client, sfwIllustID)
}

// TestPixivBinaryAuthenticatedDiscoveryCanary 验证当前源码构建的 CLI 与 MCP
// 入口。MCP 只读取公开搜索结果，任何测试输出均沿用本机凭据模式的脱敏策略。
func TestPixivBinaryAuthenticatedDiscoveryCanary(t *testing.T) {
	auth := requireAuthenticatedDiscoveryCanary(t)
	if err := validateLocalAuthCanaryBinary(auth.kind, os.Getenv("PIXIV_E2E_BINARY")); err != nil {
		t.Fatal(err)
	}
	words, err := authenticatedCanarySearchWordsFromEnvironment(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	sfwIllustID, err := authenticatedCanarySFWIllustIDFromEnvironment(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := ".."
	binaryPath := buildPixivBinary(t, repoRoot)
	var env []string
	if auth.kind == canaryAuthLocalStore {
		env = localAuthCanaryEnv()
	} else {
		env = isolatedEnv(t).values
	}
	proxy := firstNonEmpty(os.Getenv("PIXIV_E2E_PROXY"), os.Getenv("PIXIV_WEB_API_PROXY"))
	env = authenticatedCanaryChildEnvFrom(env, auth, proxy)

	novelOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "novel", "search", words.discovery, "--limit", "1", "--json")
	var novelDocument struct {
		Novels []pixiv.Novel `json:"novels"`
	}
	requireCanaryJSON(t, novelOut, auth, &novelDocument)
	if len(novelDocument.Novels) == 0 || novelDocument.Novels[0].ID <= 0 {
		t.Fatalf("CLI novel search %q returned no usable results", words.discovery)
	}

	userOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "user", "search", words.discovery, "--limit", "1", "--json")
	var userDocument struct {
		Source       pixiv.UserSearchSource `json:"source"`
		UserPreviews []pixiv.UserPreview    `json:"user_previews"`
	}
	requireCanaryJSON(t, userOut, auth, &userDocument)
	if userDocument.Source != pixiv.UserSearchSourceApp || len(userDocument.UserPreviews) == 0 || userDocument.UserPreviews[0].User.ID <= 0 {
		t.Fatalf("CLI user search source=%q results=%d, want App source and a usable result", userDocument.Source, len(userDocument.UserPreviews))
	}

	mcpNovel := callAuthenticatedDiscoveryMCP(t, repoRoot, binaryPath, env, auth, "search_novel", map[string]any{"word": words.discovery, "limit": 1})
	var mcpNovelOut struct {
		Novels []pixiv.Novel `json:"novels"`
	}
	if err := json.Unmarshal(mcpNovel, &mcpNovelOut); err != nil {
		t.Fatalf("decode MCP novel search output: %v", err)
	}
	if len(mcpNovelOut.Novels) == 0 || mcpNovelOut.Novels[0].ID <= 0 {
		t.Fatalf("MCP novel search %q returned no usable results", words.discovery)
	}

	mcpUser := callAuthenticatedDiscoveryMCP(t, repoRoot, binaryPath, env, auth, "search_user", map[string]any{"word": words.discovery, "limit": 1})
	var mcpUserOut struct {
		Source       pixiv.UserSearchSource `json:"source"`
		UserPreviews []pixiv.UserPreview    `json:"user_previews"`
	}
	if err := json.Unmarshal(mcpUser, &mcpUserOut); err != nil {
		t.Fatalf("decode MCP user search output: %v", err)
	}
	if mcpUserOut.Source != pixiv.UserSearchSourceApp || len(mcpUserOut.UserPreviews) == 0 || mcpUserOut.UserPreviews[0].User.ID <= 0 {
		t.Fatalf("MCP user search source=%q results=%d, want App source and a usable result", mcpUserOut.Source, len(mcpUserOut.UserPreviews))
	}
	requireAuthenticatedCanaryAuthorCLIAndMCP(t, repoRoot, binaryPath, env, auth, sfwIllustID)
}

func requireAuthenticatedCanaryAuthorSDK(t *testing.T, client *pixiv.Client, sfwIllustID int64) {
	t.Helper()
	detail, err := client.IllustDetail(testCommandContext(t), sfwIllustID)
	if err != nil {
		t.Fatalf("authenticated source illustration detail %d failed: %v", sfwIllustID, err)
	}
	if detail == nil || detail.Illust.ID != sfwIllustID || detail.Illust.User.ID <= 0 {
		t.Fatalf("authenticated source illustration %d has no usable author: %+v", sfwIllustID, detail)
	}
	authorID := detail.Illust.User.ID
	author, err := client.UserDetail(testCommandContext(t), pixiv.UserDetailRequest{UserID: authorID})
	if err != nil {
		t.Fatalf("authenticated author detail %d failed: %v", authorID, err)
	}
	if author == nil {
		t.Fatalf("authenticated author detail %d returned no result", authorID)
	}
	if author.User.ID != authorID {
		t.Fatalf("authenticated author detail ID=%d, want %d", author.User.ID, authorID)
	}
	artworks, err := client.UserArtworks(testCommandContext(t), pixiv.UserArtworksRequest{UserID: authorID})
	if err != nil {
		t.Fatalf("authenticated author artworks %d failed: %v", authorID, err)
	}
	if artworks == nil || len(artworks.Illusts) == 0 {
		t.Fatalf("authenticated author artworks %d returned no works", authorID)
	}
	for _, illust := range artworks.Illusts {
		if illust.User.ID != authorID {
			t.Fatalf("authenticated author artworks %d returned illustration %d owned by %d", authorID, illust.ID, illust.User.ID)
		}
	}
}

func requireAuthenticatedCanaryAuthorCLIAndMCP(t *testing.T, repoRoot, binaryPath string, env []string, auth authenticatedCanaryAuth, sfwIllustID int64) {
	t.Helper()
	sourceOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "detail", strconv.FormatInt(sfwIllustID, 10), "--json")
	var source pixiv.IllustDetail
	requireCanaryJSON(t, sourceOut, auth, &source)
	if source.Illust.ID != sfwIllustID || source.Illust.User.ID <= 0 {
		t.Fatalf("CLI source illustration %d has no usable author", sfwIllustID)
	}
	authorID := source.Illust.User.ID

	detailOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "user", "detail", strconv.FormatInt(authorID, 10), "--json")
	var author pixiv.UserDetailResult
	requireCanaryJSON(t, detailOut, auth, &author)
	if author.User.ID != authorID {
		t.Fatalf("CLI author detail ID=%d, want %d", author.User.ID, authorID)
	}
	artworksOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "user", "artworks", strconv.FormatInt(authorID, 10), "--limit", "1", "--json")
	var artworks struct {
		Illusts []pixiv.Illust `json:"illusts"`
	}
	requireCanaryJSON(t, artworksOut, auth, &artworks)
	if len(artworks.Illusts) == 0 || artworks.Illusts[0].User.ID != authorID {
		t.Fatalf("CLI author artworks %d returned no matching work", authorID)
	}

	mcpDetail := callAuthenticatedDiscoveryMCP(t, repoRoot, binaryPath, env, auth, "user_detail", map[string]any{"user_id": authorID})
	var mcpAuthor pixiv.UserDetailResult
	if err := json.Unmarshal(mcpDetail, &mcpAuthor); err != nil {
		t.Fatalf("decode MCP author detail output: %v", err)
	}
	if mcpAuthor.User.ID != authorID {
		t.Fatalf("MCP author detail ID=%d, want %d", mcpAuthor.User.ID, authorID)
	}
	mcpArtworks := callAuthenticatedDiscoveryMCP(t, repoRoot, binaryPath, env, auth, "user_artworks", map[string]any{"user_id": authorID, "limit": 1})
	var mcpWorks struct {
		UserID int64          `json:"user_id"`
		Items  []pixiv.Illust `json:"items"`
	}
	if err := json.Unmarshal(mcpArtworks, &mcpWorks); err != nil {
		t.Fatalf("decode MCP author artworks output: %v", err)
	}
	if mcpWorks.UserID != authorID || len(mcpWorks.Items) == 0 || mcpWorks.Items[0].User.ID != authorID {
		t.Fatalf("MCP author artworks user=%d items=%d, want one matching work for %d", mcpWorks.UserID, len(mcpWorks.Items), authorID)
	}
}

func callAuthenticatedDiscoveryMCP(t *testing.T, repoRoot, binaryPath string, env []string, auth authenticatedCanaryAuth, tool string, arguments map[string]any) json.RawMessage {
	t.Helper()
	command := exec.CommandContext(testCommandContext(t), binaryPath, "mcp")
	command.Dir = repoRoot
	command.Env = env
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}

	encoder := json.NewEncoder(stdin)
	for _, request := range []any{
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "e2e", "version": "1"}}},
		map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{}},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": arguments}},
	} {
		if err := encoder.Encode(request); err != nil {
			_ = stdin.Close()
			_ = command.Wait()
			t.Fatalf("write MCP %s request: %v", tool, err)
		}
	}

	decoder := json.NewDecoder(bufio.NewReader(stdout))
	for {
		var response struct {
			ID    int `json:"id"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Result *struct {
				IsError           bool            `json:"isError"`
				StructuredContent json.RawMessage `json:"structuredContent"`
			} `json:"result"`
		}
		if err := decoder.Decode(&response); err != nil {
			_ = stdin.Close()
			_ = command.Wait()
			mcpCanaryFailure(t, auth, tool, fmt.Sprintf("read MCP response: %v", err), stderr.String())
		}
		if response.ID != 2 {
			continue
		}
		if response.Error != nil || response.Result == nil || response.Result.IsError || len(response.Result.StructuredContent) == 0 {
			message := "MCP result is invalid"
			if response.Error != nil {
				message = response.Error.Message
			}
			mcpCanaryFailure(t, auth, tool, message, stderr.String())
		}
		if err := stdin.Close(); err != nil {
			t.Fatal(err)
		}
		if err := command.Wait(); err != nil {
			mcpCanaryFailure(t, auth, tool, err.Error(), stderr.String())
		}
		return response.Result.StructuredContent
	}
}

func mcpCanaryFailure(t *testing.T, auth authenticatedCanaryAuth, tool, failure, stderr string) {
	t.Helper()
	if auth.kind == canaryAuthLocalStore {
		t.Fatalf("MCP %s failed: %s; child stderr omitted to protect local-store credentials", tool, failure)
	}
	t.Fatalf("MCP %s failed: %s\nstderr:\n%s", tool, redactCanaryDiagnostic(auth.refreshToken, failure), redactCanaryDiagnostic(auth.refreshToken, stderr))
}
