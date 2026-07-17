package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestPixivBinaryBuildsFromCmdPixivAndPrintsHelp(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	binaryPath := buildPixivBinary(t, repoRoot)

	run := exec.Command(binaryPath, "--help")
	run.Dir = repoRoot
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("pixiv --help failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "Pixiv CLI and MCP server") {
		t.Fatalf("help output did not describe pixiv CLI:\n%s", string(out))
	}
}

func TestPixivBinaryUsesAuthCommandAndRemovesAccountCommand(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	binaryPath := buildPixivBinary(t, repoRoot)

	authHelp := exec.Command(binaryPath, "auth", "--help")
	authHelp.Dir = repoRoot
	out, err := authHelp.CombinedOutput()
	if err != nil {
		t.Fatalf("pixiv auth --help failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "Manage local Pixiv authentication") {
		t.Fatalf("auth help output did not describe local authentication:\n%s", string(out))
	}
	for _, command := range []struct {
		name string
		want string
	}{
		{name: "user", want: "Query a Pixiv user"},
		{name: "bookmark", want: "Manage illustration bookmarks"},
		{name: "follow", want: "Manage followed users"},
	} {
		help := exec.Command(binaryPath, command.name, "--help")
		help.Dir = repoRoot
		out, err := help.CombinedOutput()
		if err != nil {
			t.Fatalf("pixiv %s --help failed: %v\n%s", command.name, err, string(out))
		}
		if !strings.Contains(string(out), command.want) {
			t.Fatalf("%s help missing %q:\n%s", command.name, command.want, string(out))
		}
	}

	account := exec.Command(binaryPath, "account")
	account.Dir = repoRoot
	out, err = account.CombinedOutput()
	if err == nil {
		t.Fatalf("pixiv account unexpectedly succeeded:\n%s", string(out))
	}
	if !strings.Contains(string(out), `unknown command "account"`) {
		t.Fatalf("pixiv account did not report removed command:\n%s", string(out))
	}
}

func TestPixivBinaryOfflineConfigAndMCPHelp(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	binaryPath := buildPixivBinary(t, repoRoot)
	env := isolatedEnv(t)

	// 配置路径是 stdout 协议；默认日志级别不应把普通成功操作的 INFO 诊断写入
	// stderr。即使未来新增 warning，也不能混合两个流后再判断路径。
	out, stderr := runPixivStdout(t, repoRoot, binaryPath, env.values, "config", "path")
	if bytes.Contains(stderr, []byte("level=INFO")) {
		t.Fatalf("default CLI success emitted INFO diagnostic to stderr:\n%s", string(stderr))
	}
	gotConfigPath := strings.TrimSpace(string(out))
	if !strings.HasPrefix(gotConfigPath, env.configRoot) && !strings.HasPrefix(gotConfigPath, env.home) {
		t.Fatalf("config path escaped isolated config roots:\n%s", string(out))
	}
	if !strings.HasSuffix(gotConfigPath, filepath.Join("pixiv", "config.toml")) {
		t.Fatalf("config path did not point at pixiv/config.toml:\n%s", string(out))
	}

	out, _ = runPixivStdout(t, repoRoot, binaryPath, env.values, "config", "get", "download_path")
	if strings.TrimSpace(string(out)) != "./downloads" {
		t.Fatalf("download_path default changed:\n%s", string(out))
	}
	_, _ = runPixivStdout(t, repoRoot, binaryPath, env.values, "config", "set", "output_json", "true")
	out, _ = runPixivStdout(t, repoRoot, binaryPath, env.values, "config", "get", "output_json")
	if strings.TrimSpace(string(out)) != "true" {
		t.Fatalf("config set did not persist output_json:\n%s", string(out))
	}
	_, _ = runPixivStdout(t, repoRoot, binaryPath, env.values, "config", "unset", "output_json")
	out, _ = runPixivStdout(t, repoRoot, binaryPath, env.values, "config", "get", "output_json")
	if strings.TrimSpace(string(out)) != "false" {
		t.Fatalf("config unset did not restore output_json default:\n%s", string(out))
	}

	mcpHelp := exec.Command(binaryPath, "mcp", "--help")
	mcpHelp.Dir = repoRoot
	mcpHelp.Env = env.values
	out, err := mcpHelp.CombinedOutput()
	if err != nil {
		t.Fatalf("pixiv mcp --help failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "Run the MCP stdio server") {
		t.Fatalf("mcp help output did not describe stdio server:\n%s", string(out))
	}
}

func TestPixivBinaryMCPStdioListsLegacyAndSDKTools(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	binaryPath := buildPixivBinary(t, repoRoot)
	command := exec.CommandContext(testCommandContext(t), binaryPath, "mcp")
	command.Dir = repoRoot
	command.Env = isolatedEnv(t).values
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
	for _, message := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"e2e","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	} {
		if _, err := stdin.Write([]byte(message + "\n")); err != nil {
			t.Fatal(err)
		}
	}

	scanner := bufio.NewScanner(stdout)
	for range 2 {
		if !scanner.Scan() {
			t.Fatalf("MCP server ended before responses: %v; stderr=%s", scanner.Err(), stderr.String())
		}
		line := scanner.Bytes()
		if !json.Valid(line) {
			t.Fatalf("MCP stdout is not JSON-RPC: %s", line)
		}
		var response struct {
			ID     int `json:"id"`
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatalf("decode MCP response: %v", err)
		}
		if response.ID != 2 {
			continue
		}
		names := make(map[string]struct{}, len(response.Result.Tools))
		for _, tool := range response.Result.Tools {
			names[tool.Name] = struct{}{}
		}
		for _, want := range []string{"search_illust", "user_artworks", "add_bookmark", "follow_user"} {
			if _, ok := names[want]; !ok {
				t.Fatalf("MCP tool %q missing from %+v", want, response.Result.Tools)
			}
		}
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("MCP server failed: %v; stderr=%s", err, stderr.String())
	}
}

func TestPixivBinaryUsesProvidedBinaryAndExpectedVersion(t *testing.T) {
	externalBinary := os.Getenv("PIXIV_E2E_BINARY")
	if externalBinary == "" {
		t.Skip("set PIXIV_E2E_BINARY to validate an already built binary")
	}
	expectedVersion := os.Getenv("PIXIV_E2E_EXPECTED_VERSION")
	if expectedVersion == "" {
		t.Fatal("PIXIV_E2E_EXPECTED_VERSION is required with PIXIV_E2E_BINARY")
	}

	repoRoot := filepath.Join("..", "..")
	binaryPath := buildPixivBinary(t, repoRoot)
	out, _ := runPixivStdout(t, repoRoot, binaryPath, isolatedEnv(t).values, "version", "--json")
	var version struct {
		Version string `json:"version"`
	}
	requireJSON(t, out, &version)
	if version.Version != expectedVersion {
		t.Fatalf("external binary version = %q, want %q", version.Version, expectedVersion)
	}
}

func TestPixivBinaryWebAPIFallbackReal(t *testing.T) {
	if os.Getenv("PIXIV_E2E_WEB_API") != "1" {
		t.Skip("set PIXIV_E2E_WEB_API=1 to run real Pixiv web fallback e2e")
	}

	repoRoot := filepath.Join("..", "..")
	binaryPath := buildPixivBinary(t, repoRoot)
	env := isolatedEnv(t).values
	downloadPath := t.TempDir()
	env = append(env, "PIXIV_REFRESH_TOKEN=", "DOWNLOAD_PATH="+downloadPath)
	if proxy := os.Getenv("PIXIV_WEB_API_PROXY"); proxy != "" {
		env = append(env, "https_proxy="+proxy, "HTTPS_PROXY="+proxy)
	}

	searchOut := runPixiv(t, repoRoot, binaryPath, env, "search", "初音ミク", "--json")
	var searchResult pixiv.IllustListResult
	requireJSON(t, searchOut, &searchResult)
	if len(searchResult.Illusts) == 0 {
		t.Fatalf("web fallback search returned no illustrations:\n%s", string(searchOut))
	}

	// 匿名 Web 搜索列表不包含真实宽高；必须经 detail 取得尺寸后才能从
	// baseline 选择可靠候选，不能把列表模型中的零值解释成筛选失败。
	firstAspect := ""
	aspect := ""
	for _, illust := range searchResult.Illusts {
		if illust.ID <= 0 {
			continue
		}
		detail := webCanaryIllustDetail(t, repoRoot, binaryPath, env, illust.ID)
		currentAspect := canaryAspectRatio(detail)
		if currentAspect == "" {
			continue
		}
		if firstAspect == "" {
			firstAspect = currentAspect
			aspect = currentAspect
			continue
		}
		// 优先选择不同于 baseline 首项的值；若后端忽略 ratio，limit=1
		// 会返回首项并使语义断言失败，而不是偶然变绿。
		if currentAspect != firstAspect {
			aspect = currentAspect
			break
		}
	}
	if aspect == "" {
		t.Fatal("web fallback baseline details returned no illustration with classifiable dimensions")
	}
	advancedSearchOut := runPixiv(t, repoRoot, binaryPath, env, "search", "初音ミク", "--aspect-ratio", aspect, "--limit", "1", "--json")
	advancedIllusts := requireIllustListJSONShape(t, advancedSearchOut, "anonymous web advanced search")
	if len(advancedIllusts) == 0 {
		t.Fatalf("anonymous web %s search returned no illustrations despite a matching baseline candidate", aspect)
	}
	for _, illust := range advancedIllusts {
		detail := webCanaryIllustDetail(t, repoRoot, binaryPath, env, illust.ID)
		if canaryAspectRatio(detail) != aspect {
			t.Fatalf("anonymous web %s search returned illustration %d with dimensions %dx%d", aspect, illust.ID, detail.Width, detail.Height)
		}
	}

	downloadID := int64(0)
	for _, illust := range searchResult.Illusts {
		if illust.ID > 0 && illust.Type != "ugoira" {
			downloadID = illust.ID
			break
		}
	}
	if downloadID == 0 {
		t.Fatalf("web fallback search returned no static or manga illustration candidate:\n%s", string(searchOut))
	}

	detailOut := runPixiv(t, repoRoot, binaryPath, env, "detail", strconvFormatInt(downloadID), "--json")
	var detailResult pixiv.IllustDetail
	requireJSON(t, detailOut, &detailResult)
	if detailResult.Illust.ID != downloadID {
		t.Fatalf("web fallback detail returned wrong id: got %d want %d\n%s", detailResult.Illust.ID, downloadID, string(detailOut))
	}
	if detailResult.Illust.MetaSinglePage.OriginalImageURL == "" && len(detailResult.Illust.MetaPages) == 0 {
		t.Fatalf("web fallback detail did not include downloadable page URLs:\n%s", string(detailOut))
	}

	rankingOut := runPixiv(t, repoRoot, binaryPath, env, "ranking", "--json")
	var rankingResult pixiv.IllustListResult
	requireJSON(t, rankingOut, &rankingResult)
	if len(rankingResult.Illusts) == 0 {
		t.Fatalf("web fallback ranking returned no illustrations:\n%s", string(rankingOut))
	}

	downloadOut := runPixiv(t, repoRoot, binaryPath, env, "download", strconvFormatInt(downloadID))
	if !strings.Contains(string(downloadOut), "downloaded ") {
		t.Fatalf("web fallback download did not report success:\n%s", string(downloadOut))
	}
	if countRegularFiles(t, downloadPath) == 0 {
		t.Fatalf("web fallback download reported success but wrote no files under %s", downloadPath)
	}
}

func TestPixivBinaryRealAPISearchOptIn(t *testing.T) {
	if os.Getenv("PIXIV_E2E_REAL_API") != "1" {
		t.Skip("set PIXIV_E2E_REAL_API=1 to run real Pixiv API e2e")
	}

	repoRoot := filepath.Join("..", "..")
	binaryPath := buildPixivBinary(t, repoRoot)
	env := isolatedEnv(t).values
	if proxy := firstNonEmpty(os.Getenv("PIXIV_E2E_PROXY"), os.Getenv("PIXIV_WEB_API_PROXY")); proxy != "" {
		env = append(env, "https_proxy="+proxy, "HTTPS_PROXY="+proxy)
	}
	refreshToken := os.Getenv("PIXIV_E2E_REFRESH_TOKEN")
	if refreshToken != "" {
		env = append(env, "PIXIV_REFRESH_TOKEN="+refreshToken)
	} else {
		env = append(env, "PIXIV_REFRESH_TOKEN=")
	}

	if refreshToken == "" {
		run := exec.CommandContext(testCommandContext(t), binaryPath, "search", "初音ミク")
		run.Dir = repoRoot
		run.Env = env
		out, err := run.CombinedOutput()
		if err != nil {
			t.Fatalf("unauthenticated real web fallback search failed: %v\n%s", err, string(out))
		}
		if !strings.Contains(string(out), "found ") {
			t.Fatalf("unauthenticated real web fallback search did not print result summary:\n%s", string(out))
		}
		return
	}

	// App API 认证失败时，stdout/stderr 都可能带上服务端回显；统一交给
	// canary helper 分流并在测试诊断前脱敏，避免显式注入的 token 泄露。
	stdout := runPixivCanary(t, repoRoot, binaryPath, env, authenticatedCanaryAuth{kind: canaryAuthExplicitToken, refreshToken: refreshToken}, "search", "初音ミク")
	if !strings.Contains(string(stdout), "found ") {
		t.Fatalf("real API search did not print result summary:\n%s", redactCanaryDiagnostic(refreshToken, string(stdout)))
	}
}

func TestResolveAuthenticatedCanaryAuth(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		refreshToken string
		useLocalAuth string
		wantKind     canaryAuthKind
		wantSkip     bool
		wantError    bool
	}{
		{name: "neither credential source skips", wantSkip: true},
		{name: "explicit temporary token", refreshToken: "temporary-token", wantKind: canaryAuthExplicitToken},
		{name: "explicit local auth", useLocalAuth: "1", wantKind: canaryAuthLocalStore},
		{name: "both sources are rejected", refreshToken: "temporary-token", useLocalAuth: "1", wantError: true},
		{name: "invalid local auth value is rejected", useLocalAuth: "true", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth, skip, err := resolveAuthenticatedCanaryAuth(test.refreshToken, test.useLocalAuth)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, want error=%t", err, test.wantError)
			}
			if skip != test.wantSkip {
				t.Fatalf("skip = %t, want %t", skip, test.wantSkip)
			}
			if !test.wantError && !test.wantSkip && auth.kind != test.wantKind {
				t.Fatalf("auth kind = %d, want %d", auth.kind, test.wantKind)
			}
		})
	}
}

func TestCanarySearchCandidateClassifiers(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		illust     pixiv.Illust
		resolution string
		aspect     string
		content    string
	}{
		{name: "high landscape illustration", illust: pixiv.Illust{Width: 4000, Height: 3000, Type: "illust"}, resolution: "high", aspect: "landscape", content: "illust"},
		{name: "medium portrait manga", illust: pixiv.Illust{Width: 1000, Height: 2999, Type: "manga"}, resolution: "medium", aspect: "portrait", content: "manga"},
		{name: "low square ugoira", illust: pixiv.Illust{Width: 999, Height: 999, Type: "ugoira"}, resolution: "low", aspect: "square", content: "ugoira"},
		{name: "dimensions straddle official buckets", illust: pixiv.Illust{Width: 3000, Height: 2999, Type: "novel"}, aspect: "landscape"},
		{name: "missing dimensions", illust: pixiv.Illust{Type: "illust"}, content: "illust"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := canaryResolution(test.illust); got != test.resolution {
				t.Fatalf("resolution = %q, want %q", got, test.resolution)
			}
			if got := canaryAspectRatio(test.illust); got != test.aspect {
				t.Fatalf("aspect ratio = %q, want %q", got, test.aspect)
			}
			if got := canaryContentType(test.illust); got != test.content {
				t.Fatalf("content type = %q, want %q", got, test.content)
			}
		})
	}
}

func TestCanaryToolCandidateUsesOptionsAndBaselineIntersection(t *testing.T) {
	t.Parallel()

	illusts := []pixiv.Illust{
		{Tools: []string{"PaintTool SAI"}},
		{Tools: []string{"CLIP STUDIO PAINT", "Photoshop"}},
	}
	if got, ok := canaryToolCandidate([]string{"Photoshop", "CLIP STUDIO PAINT"}, illusts); !ok || got != "Photoshop" {
		t.Fatalf("candidate = %q, %t, want Photoshop, true", got, ok)
	}
	if got, ok := canaryToolCandidate([]string{"Procreate"}, illusts); ok || got != "" {
		t.Fatalf("candidate = %q, %t, want empty, false", got, ok)
	}
	if got, ok := canaryToolCandidate([]string{"PaintTool SAI"}, illusts); !ok || got != "PaintTool SAI" {
		t.Fatalf("fallback candidate = %q, %t, want PaintTool SAI, true", got, ok)
	}
}

func TestCanaryFilterCandidatePrefersValueDifferentFromBaselineFirstItem(t *testing.T) {
	t.Parallel()

	illusts := []pixiv.Illust{
		{Width: 1200, Height: 1800, Type: "illust"},
		{Width: 4000, Height: 3000, Type: "manga"},
	}
	if got := canaryFilterCandidateValue(illusts, canaryResolution); got != "high" {
		t.Fatalf("resolution candidate = %q, want high", got)
	}
	if got := canaryFilterCandidateValue(illusts, canaryAspectRatio); got != "landscape" {
		t.Fatalf("aspect candidate = %q, want landscape", got)
	}
	if got := canaryFilterCandidateValue(illusts, canaryContentType); got != "manga" {
		t.Fatalf("content candidate = %q, want manga", got)
	}
}

func TestLocalAuthCanaryEnvironmentRemovesRefreshTokenOverride(t *testing.T) {
	t.Parallel()

	env := localAuthCanaryEnvFrom([]string{
		"HOME=/actual-home",
		"XDG_CONFIG_HOME=/actual-config",
		"PIXIV_REFRESH_TOKEN=must-not-reach-child",
		"PATH=/bin",
	})
	if slices.Contains(env, "PIXIV_REFRESH_TOKEN=must-not-reach-child") {
		t.Fatal("local auth canary inherited PIXIV_REFRESH_TOKEN")
	}
	for _, required := range []string{"HOME=/actual-home", "XDG_CONFIG_HOME=/actual-config", "PATH=/bin"} {
		if !slices.Contains(env, required) {
			t.Fatalf("local auth canary environment lost %q", required)
		}
	}
}

func TestLocalAuthCanaryRejectsExternalBinary(t *testing.T) {
	t.Parallel()

	if err := validateLocalAuthCanaryBinary(canaryAuthLocalStore, "/untrusted/pixiv"); err == nil {
		t.Fatal("local-auth canary accepted PIXIV_E2E_BINARY")
	}
	if err := validateLocalAuthCanaryBinary(canaryAuthLocalStore, ""); err != nil {
		t.Fatalf("local-auth canary rejected source build: %v", err)
	}
	if err := validateLocalAuthCanaryBinary(canaryAuthExplicitToken, "/release/pixiv"); err != nil {
		t.Fatalf("explicit-token canary unexpectedly rejected external binary: %v", err)
	}
}

func TestRedactCanaryDiagnostic(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		token  string
		input  string
		secret string
	}{
		{name: "explicit token", token: "temporary-token", input: "failure temporary-token", secret: "temporary-token"},
		{name: "query-style refresh token", input: "refresh_token=local-token", secret: "local-token"},
		{name: "JSON refresh token", input: `{"refresh_token":"local-token"}`, secret: "local-token"},
		{name: "query-style access token", input: "access_token=local-access-token", secret: "local-access-token"},
		{name: "JSON access token", input: `{"access_token":"local-access-token"}`, secret: "local-access-token"},
		{name: "cookie header", input: "Cookie: PHPSESSID=local-cookie; other=value", secret: "local-cookie"},
		{name: "set-cookie header", input: "Set-Cookie: session=local-cookie; Secure", secret: "local-cookie"},
		{name: "bearer authorization", input: "Authorization: Bearer local-token", secret: "local-token"},
		{name: "basic authorization", input: "Authorization: Basic bG9jYWwtdG9rZW4=", secret: "bG9jYWwtdG9rZW4="},
		{name: "OAuth code query", input: "https://example.test/callback?code=oauth-code&state=state", secret: "oauth-code"},
		{name: "OAuth code verifier JSON", input: `{"code_verifier":"oauth-verifier"}`, secret: "oauth-verifier"},
	} {
		t.Run(test.name, func(t *testing.T) {
			redacted := redactCanaryDiagnostic(test.token, test.input)
			if strings.Contains(redacted, test.secret) {
				t.Fatalf("diagnostic leaked credential: %q", redacted)
			}
			if !strings.Contains(redacted, "[redacted]") {
				t.Fatalf("diagnostic did not show redaction: %q", redacted)
			}
		})
	}
}

func TestLocalStoreCanaryFailureDiagnosticsOmitChildOutput(t *testing.T) {
	t.Parallel()

	const unknownLocalToken = "unknown-local-token"
	diagnostic := canaryFailureDiagnostics(
		authenticatedCanaryAuth{kind: canaryAuthLocalStore},
		"exit status 1",
		"oauth body refresh_token="+unknownLocalToken,
		"Authorization: Bearer "+unknownLocalToken,
	)
	if strings.Contains(diagnostic, unknownLocalToken) {
		t.Fatalf("local-store diagnostic leaked credential: %q", diagnostic)
	}
	if !strings.Contains(diagnostic, "omitted") {
		t.Fatalf("local-store diagnostic did not explain output omission: %q", diagnostic)
	}
}

func TestExplicitTokenCanaryFailureDiagnosticsRedactKnownToken(t *testing.T) {
	t.Parallel()

	const explicitToken = "explicit-test-token"
	diagnostic := canaryFailureDiagnostics(
		authenticatedCanaryAuth{kind: canaryAuthExplicitToken, refreshToken: explicitToken},
		"exit status 1",
		"oauth body "+explicitToken,
		"server echoed "+explicitToken,
	)
	if strings.Contains(diagnostic, explicitToken) {
		t.Fatalf("explicit-token diagnostic leaked credential: %q", diagnostic)
	}
}

// TestPixivBinaryAuthenticatedAppAPICanary 覆盖需要登录态的稳定公开 SDK
// 能力。调用者必须明确选择隔离的临时 token 或本机持久化账号；默认绝不复用
// 本机认证配置，避免误把日常开发环境的凭据用于联网发布门禁。
func TestPixivBinaryAuthenticatedAppAPICanary(t *testing.T) {
	if os.Getenv("PIXIV_E2E_REAL_API") != "1" {
		t.Skip("set PIXIV_E2E_REAL_API=1 to run authenticated App API canary")
	}
	auth, skip, err := resolveAuthenticatedCanaryAuth(os.Getenv("PIXIV_E2E_REFRESH_TOKEN"), os.Getenv("PIXIV_E2E_USE_LOCAL_AUTH"))
	if err != nil {
		t.Fatalf("invalid authenticated App API canary credential mode: %v", err)
	}
	if skip {
		t.Skip("set PIXIV_E2E_REFRESH_TOKEN or PIXIV_E2E_USE_LOCAL_AUTH=1 with PIXIV_E2E_REAL_API=1 to run authenticated App API canary")
	}
	if err := validateLocalAuthCanaryBinary(auth.kind, os.Getenv("PIXIV_E2E_BINARY")); err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join("..", "..")
	binaryPath := buildPixivBinary(t, repoRoot)
	var env []string
	if auth.kind == canaryAuthLocalStore {
		// 本机模式必须沿 CLI 的默认 store 路径刷新并持久化 token，不能让父进程
		// 的运行期覆盖把 rotation 留在内存中。
		env = localAuthCanaryEnv()
	} else {
		env = isolatedEnv(t).values
		env = append(env, "PIXIV_REFRESH_TOKEN="+auth.refreshToken)
	}
	if proxy := firstNonEmpty(os.Getenv("PIXIV_E2E_PROXY"), os.Getenv("PIXIV_WEB_API_PROXY")); proxy != "" {
		env = append(env, "https_proxy="+proxy, "HTTPS_PROXY="+proxy)
	}

	accountOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "auth", "check", "--json")
	var account struct {
		UserID int64 `json:"user_id"`
	}
	requireCanaryJSON(t, accountOut, auth, &account)
	if account.UserID <= 0 {
		t.Fatalf("auth check returned invalid user_id: %d", account.UserID)
	}

	const searchWord = "初音ミク"
	baselineOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "search", searchWord, "--json")
	baselineIllusts := requireCanaryIllustListJSON(t, baselineOut, auth)
	if len(baselineIllusts) == 0 {
		t.Fatal("authenticated App search baseline returned no illustrations")
	}

	optionsOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "search-options", searchWord, "--json")
	var options pixiv.SearchIllustOptionsResult
	requireCanaryJSON(t, optionsOut, auth, &options)
	if options.Tools == nil {
		t.Fatal("search options returned a null or missing tools array")
	}
	resolution := canaryFilterCandidateValue(baselineIllusts, canaryResolution)
	if resolution == "" {
		t.Fatal("authenticated App baseline returned no illustration in an official resolution bucket")
	}
	aspect := canaryFilterCandidateValue(baselineIllusts, canaryAspectRatio)
	if aspect == "" {
		t.Fatal("authenticated App baseline returned no illustration with classifiable dimensions")
	}
	contentType := canaryFilterCandidateValue(baselineIllusts, canaryContentType)
	if contentType == "" {
		t.Fatal("authenticated App baseline returned no illustration with a supported content type")
	}
	if !slices.ContainsFunc(baselineIllusts, func(illust pixiv.Illust) bool { return illust.AIType != 2 }) {
		t.Fatal("authenticated App baseline returned no non-AI illustration candidate")
	}

	for _, search := range []struct {
		name     string
		args     []string
		validate func(pixiv.Illust) bool
		want     string
	}{
		{
			name:     "resolution-" + resolution,
			args:     []string{"search", searchWord, "--resolution", resolution, "--limit", "1", "--json"},
			validate: func(illust pixiv.Illust) bool { return canaryResolution(illust) == resolution },
			want:     "official " + resolution + " resolution bucket",
		},
		{
			name:     "aspect-" + aspect,
			args:     []string{"search", searchWord, "--aspect-ratio", aspect, "--limit", "1", "--json"},
			validate: func(illust pixiv.Illust) bool { return canaryAspectRatio(illust) == aspect },
			want:     aspect + " aspect ratio",
		},
		{
			name:     "content-type-" + contentType,
			args:     []string{"search", searchWord, "--type", contentType, "--limit", "1", "--json"},
			validate: func(illust pixiv.Illust) bool { return canaryContentType(illust) == contentType },
			want:     "content type " + contentType,
		},
		{
			name:     "exclude-ai",
			args:     []string{"search", searchWord, "--ai-mode", "exclude", "--limit", "1", "--json"},
			validate: func(illust pixiv.Illust) bool { return illust.AIType != 2 },
			want:     "ai_type != 2",
		},
	} {
		t.Run("search-filter/"+search.name, func(t *testing.T) {
			out := runPixivCanary(t, repoRoot, binaryPath, env, auth, search.args...)
			illusts := requireCanaryIllustListJSON(t, out, auth)
			if len(illusts) == 0 {
				t.Fatalf("search filter %s returned no illustrations despite a matching baseline candidate", search.name)
			}
			for _, illust := range illusts {
				if !search.validate(illust) {
					t.Fatalf("search filter %s returned illustration %d that does not satisfy %s", search.name, illust.ID, search.want)
				}
			}
		})
	}

	selectedTool, ok := canaryToolCandidate(options.Tools, baselineIllusts)
	if !ok {
		t.Fatal("search options and authenticated baseline returned no shared tool candidate")
	}
	toolOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "search", searchWord, "--tool", selectedTool, "--limit", "1", "--json")
	toolIllusts := requireCanaryIllustListJSON(t, toolOut, auth)
	if len(toolIllusts) == 0 {
		t.Fatal("tool-filter search returned no illustrations despite a matching baseline candidate")
	}
	for _, illust := range toolIllusts {
		if !slices.Contains(illust.Tools, selectedTool) {
			t.Fatalf("tool-filter search returned illustration %d without the selected tool", illust.ID)
		}
	}

	userDetailOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "user", "detail", strconvFormatInt(account.UserID), "--json")
	var detail pixiv.UserDetailResult
	requireCanaryJSON(t, userDetailOut, auth, &detail)
	if detail.User.ID != account.UserID {
		t.Fatalf("user detail returned user id %d, want %d", detail.User.ID, account.UserID)
	}
	requireCanaryVisibleJSONFields(t, userDetailOut, auth, "user", "profile", "profile_publicity", "workspace")

	for _, recommendation := range []struct {
		kind  string
		field string
	}{
		{kind: "illust", field: "illusts"},
		{kind: "manga", field: "manga"},
		{kind: "novel", field: "novels"},
		{kind: "user", field: "user_previews"},
	} {
		t.Run(recommendation.kind, func(t *testing.T) {
			out := runPixivCanary(t, repoRoot, binaryPath, env, auth, "recommended", recommendation.kind, "--limit", "1", "--json")
			requireCanaryJSONArrayField(t, out, auth, recommendation.field)
			requireNoCanaryContinuationFields(t, out)
		})
	}

	allOut := runPixivCanary(t, repoRoot, binaryPath, env, auth, "recommended", "all", "--limit", "1", "--json")
	for _, field := range []string{"illusts", "manga", "novels", "user_previews"} {
		requireCanaryJSONArrayField(t, allOut, auth, field)
	}
	requireNoCanaryContinuationFields(t, allOut)
}

type canaryAuthKind uint8

const (
	canaryAuthNone canaryAuthKind = iota
	canaryAuthExplicitToken
	canaryAuthLocalStore
)

type authenticatedCanaryAuth struct {
	kind         canaryAuthKind
	refreshToken string
}

// resolveAuthenticatedCanaryAuth 强制认证来源互斥，避免运行期 token 覆盖本机
// store 后，使 App API rotation 未能按正常 CLI 路径持久化。
func resolveAuthenticatedCanaryAuth(refreshToken, useLocalAuth string) (authenticatedCanaryAuth, bool, error) {
	if useLocalAuth != "" && useLocalAuth != "1" {
		return authenticatedCanaryAuth{}, false, errors.New("PIXIV_E2E_USE_LOCAL_AUTH must be exactly 1 when set")
	}
	if refreshToken != "" && useLocalAuth == "1" {
		return authenticatedCanaryAuth{}, false, errors.New("PIXIV_E2E_REFRESH_TOKEN and PIXIV_E2E_USE_LOCAL_AUTH=1 are mutually exclusive")
	}
	if refreshToken != "" {
		return authenticatedCanaryAuth{kind: canaryAuthExplicitToken, refreshToken: refreshToken}, false, nil
	}
	if useLocalAuth == "1" {
		return authenticatedCanaryAuth{kind: canaryAuthLocalStore}, false, nil
	}
	return authenticatedCanaryAuth{kind: canaryAuthNone}, true, nil
}

// validateLocalAuthCanaryBinary 拒绝将本机长期凭据交给任意外部 binary。
// 显式 token 模式仍可用于 release archive 的离线二进制验证；本机模式则仅运行
// 当前工作树构建出的 CLI，避免错误或恶意二进制读取持久化账号 store。
func validateLocalAuthCanaryBinary(kind canaryAuthKind, externalBinary string) error {
	if kind == canaryAuthLocalStore && externalBinary != "" {
		return errors.New("PIXIV_E2E_BINARY is not permitted with PIXIV_E2E_USE_LOCAL_AUTH=1; unset it to build and test the current source")
	}
	return nil
}

// localAuthCanaryEnv 保留调用者的 HOME/XDG 配置，使子 CLI 使用默认账号 store。
// 唯一刻意移除的覆盖是 PIXIV_REFRESH_TOKEN，避免跳过正常 rotation 持久化路径。
func localAuthCanaryEnv() []string {
	return localAuthCanaryEnvFrom(os.Environ())
}

func localAuthCanaryEnvFrom(environ []string) []string {
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(name, "PIXIV_REFRESH_TOKEN") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// runPixivCanary 在显式 token 模式保留经过脱敏的诊断；本机 store 模式没有可
// 精确匹配的长期凭据，因此失败时绝不回显子进程输出。
func runPixivCanary(t *testing.T, repoRoot, binaryPath string, env []string, auth authenticatedCanaryAuth, args ...string) []byte {
	t.Helper()

	run := exec.CommandContext(testCommandContext(t), binaryPath, args...)
	run.Dir = repoRoot
	run.Env = env
	var stdout, stderr bytes.Buffer
	run.Stdout = &stdout
	run.Stderr = &stderr
	err := run.Run()
	if err != nil {
		failure := err.Error()
		if auth.kind == canaryAuthLocalStore {
			failure = canaryCommandFailureSummary(err)
		}
		t.Fatalf("pixiv %s failed: %s", strings.Join(args, " "), canaryFailureDiagnostics(auth, failure, stdout.String(), stderr.String()))
	}
	return stdout.Bytes()
}

func canaryCommandFailureSummary(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ProcessState != nil {
		return exitErr.ProcessState.String()
	}
	return "child process could not be started"
}

func canaryFailureDiagnostics(auth authenticatedCanaryAuth, failure, stdout, stderr string) string {
	if auth.kind == canaryAuthLocalStore {
		return failure + "; child stdout/stderr omitted to protect local-store credentials"
	}
	return fmt.Sprintf("%s\nstdout:\n%s\nstderr:\n%s", redactCanaryDiagnostic(auth.refreshToken, failure), redactCanaryDiagnostic(auth.refreshToken, stdout), redactCanaryDiagnostic(auth.refreshToken, stderr))
}

func requireCanaryJSON(t *testing.T, body []byte, auth authenticatedCanaryAuth, out any) {
	t.Helper()

	if err := json.Unmarshal(body, out); err != nil {
		if auth.kind == canaryAuthLocalStore {
			t.Fatalf("decode local-store canary JSON failed: %v; response body omitted to protect local-store credentials", err)
		}
		t.Fatalf("decode canary JSON failed: %v\n%s", err, redactCanaryDiagnostic(auth.refreshToken, string(body)))
	}
}

func requireCanaryIllustListJSON(t *testing.T, body []byte, auth authenticatedCanaryAuth) []pixiv.Illust {
	t.Helper()

	var document map[string]json.RawMessage
	requireCanaryJSON(t, body, auth, &document)
	raw, ok := document["illusts"]
	if !ok || bytes.Equal(raw, []byte("null")) {
		t.Fatal(`canary JSON field "illusts" is missing or null`)
	}
	var illusts []pixiv.Illust
	if err := json.Unmarshal(raw, &illusts); err != nil {
		t.Fatalf(`canary JSON field "illusts" is not an illustration array: %v`, err)
	}
	return illusts
}

func redactCanaryDiagnostic(refreshToken, value string) string {
	if refreshToken != "" {
		value = strings.ReplaceAll(value, refreshToken, "[redacted]")
	}
	// 本机 store 模式没有可直接比对的 token。CLI/SDK 本身应当脱敏错误，仍在
	// 测试框架输出前清理常见的凭据键值形式，避免上游服务意外回显认证材料。
	for _, pattern := range canaryCredentialPatterns {
		value = pattern.ReplaceAllString(value, "$1[redacted]")
	}
	return value
}

var canaryCredentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^((?:set-)?cookie\s*:\s*)[^\r\n]*`),
	regexp.MustCompile(`(?im)^(authorization\s*:\s*(?:bearer|basic)\s+)[^\r\n]*`),
	regexp.MustCompile(`(?i)((?:"(?:refresh_token|access_token|code_verifier|code)"\s*:\s*"))[^"]*`),
	regexp.MustCompile(`(?i)((?:refresh_token|access_token|code_verifier|code)\s*=\s*)[^&\s,"'}\]]+`),
}

func requireCanaryVisibleJSONFields(t *testing.T, body []byte, auth authenticatedCanaryAuth, fields ...string) {
	t.Helper()

	var document map[string]json.RawMessage
	requireCanaryJSON(t, body, auth, &document)
	for _, field := range fields {
		value, ok := document[field]
		if !ok || len(value) == 0 || bytes.Equal(value, []byte("null")) {
			t.Fatalf("JSON field %q is missing or null", field)
		}
	}
}

func requireCanaryJSONArrayField(t *testing.T, body []byte, auth authenticatedCanaryAuth, field string) {
	t.Helper()

	var document map[string]json.RawMessage
	requireCanaryJSON(t, body, auth, &document)
	value, ok := document[field]
	if !ok || bytes.Equal(value, []byte("null")) {
		t.Fatalf("JSON field %q is missing or null", field)
	}
	var array []json.RawMessage
	if err := json.Unmarshal(value, &array); err != nil {
		t.Fatalf("JSON field %q is not an array: %v", field, err)
	}
}

// requireNoCanaryContinuationFields 断言公开推荐输出没有泄露上游分页协议字段。
func requireNoCanaryContinuationFields(t *testing.T, body []byte) {
	t.Helper()

	for _, forbidden := range [][]byte{[]byte(`"cursor"`), []byte(`"next_cursor"`), []byte(`"next_url"`)} {
		if bytes.Contains(body, forbidden) {
			t.Fatalf("recommended output leaked upstream continuation field %s", forbidden)
		}
	}
}

func runPixiv(t *testing.T, repoRoot, binaryPath string, env []string, args ...string) []byte {
	t.Helper()

	run := exec.CommandContext(testCommandContext(t), binaryPath, args...)
	run.Dir = repoRoot
	run.Env = env
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("pixiv %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return out
}

func webCanaryIllustDetail(t *testing.T, repoRoot, binaryPath string, env []string, illustID int64) pixiv.Illust {
	t.Helper()

	out := runPixiv(t, repoRoot, binaryPath, env, "detail", strconvFormatInt(illustID), "--json")
	var detail pixiv.IllustDetail
	requireJSON(t, out, &detail)
	if detail.Illust.ID != illustID {
		t.Fatalf("anonymous web detail returned illustration %d, want %d", detail.Illust.ID, illustID)
	}
	return detail.Illust
}

func runPixivStdout(t *testing.T, repoRoot, binaryPath string, env []string, args ...string) ([]byte, []byte) {
	t.Helper()

	run := exec.CommandContext(testCommandContext(t), binaryPath, args...)
	run.Dir = repoRoot
	run.Env = env
	var stdout, stderr bytes.Buffer
	run.Stdout = &stdout
	run.Stderr = &stderr
	if err := run.Run(); err != nil {
		t.Fatalf("pixiv %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.Bytes(), stderr.Bytes()
}

func requireJSON(t *testing.T, body []byte, out any) {
	t.Helper()

	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode JSON failed: %v\n%s", err, string(body))
	}
}

func requireIllustListJSONShape(t *testing.T, body []byte, operation string) []pixiv.Illust {
	t.Helper()

	var document map[string]json.RawMessage
	requireJSON(t, body, &document)
	raw, ok := document["illusts"]
	if !ok || bytes.Equal(raw, []byte("null")) {
		t.Fatalf("%s JSON field %q is missing or null", operation, "illusts")
	}
	var illusts []pixiv.Illust
	if err := json.Unmarshal(raw, &illusts); err != nil {
		t.Fatalf("%s JSON field %q is not an illustration array: %v", operation, "illusts", err)
	}
	return illusts
}

func canaryFilterCandidateValue(illusts []pixiv.Illust, classify func(pixiv.Illust) string) string {
	firstValue := ""
	if len(illusts) > 0 {
		firstValue = classify(illusts[0])
	}
	fallback := ""
	for _, illust := range illusts {
		value := classify(illust)
		if value == "" {
			continue
		}
		if fallback == "" {
			fallback = value
		}
		if value != firstValue {
			return value
		}
	}
	return fallback
}

// canaryResolution 只返回官方三档能够完整表达的尺寸。跨档作品不是任何
// resolution flag 的可靠候选，不能用来证明服务端筛选生效。
func canaryResolution(illust pixiv.Illust) string {
	switch {
	case illust.Width >= 3000 && illust.Height >= 3000:
		return "high"
	case illust.Width >= 1000 && illust.Width <= 2999 && illust.Height >= 1000 && illust.Height <= 2999:
		return "medium"
	case illust.Width > 0 && illust.Width <= 999 && illust.Height > 0 && illust.Height <= 999:
		return "low"
	default:
		return ""
	}
}

func canaryAspectRatio(illust pixiv.Illust) string {
	if illust.Width <= 0 || illust.Height <= 0 {
		return ""
	}
	switch {
	case illust.Width > illust.Height:
		return "landscape"
	case illust.Width < illust.Height:
		return "portrait"
	default:
		return "square"
	}
}

func canaryContentType(illust pixiv.Illust) string {
	switch illust.Type {
	case "illust", "manga", "ugoira":
		return illust.Type
	default:
		return ""
	}
}

func canaryToolCandidate(options []string, illusts []pixiv.Illust) (string, bool) {
	fallback := ""
	for _, option := range options {
		if option == "" {
			continue
		}
		for _, illust := range illusts {
			if slices.Contains(illust.Tools, option) {
				if fallback == "" {
					fallback = option
				}
				if !slices.Contains(illusts[0].Tools, option) {
					return option, true
				}
				break
			}
		}
	}
	if fallback != "" {
		return fallback, true
	}
	return "", false
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func countRegularFiles(t *testing.T, root string) int {
	t.Helper()

	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("walk downloaded files failed: %v", err)
	}
	return count
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func buildPixivBinary(t *testing.T, repoRoot string) string {
	t.Helper()
	if externalBinary := os.Getenv("PIXIV_E2E_BINARY"); externalBinary != "" {
		info, err := os.Stat(externalBinary)
		if err != nil {
			t.Fatalf("stat PIXIV_E2E_BINARY: %v", err)
		}
		if info.IsDir() {
			t.Fatalf("PIXIV_E2E_BINARY is a directory: %s", externalBinary)
		}
		return externalBinary
	}

	binaryName := "pixiv"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/pixiv")
	build.Dir = repoRoot
	var buildErr bytes.Buffer
	build.Stderr = &buildErr
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/pixiv failed: %v\n%s", err, buildErr.String())
	}
	return binaryPath
}

type isolatedProcessEnv struct {
	values     []string
	home       string
	configRoot string
}

func isolatedEnv(t *testing.T) isolatedProcessEnv {
	t.Helper()

	home := t.TempDir()
	configRoot := filepath.Join(t.TempDir(), "config")
	filtered := make([]string, 0, len(os.Environ())+7)
	for _, entry := range os.Environ() {
		name, _, found := strings.Cut(entry, "=")
		if found && isIsolatedEnvKey(name) {
			continue
		}
		filtered = append(filtered, entry)
	}
	filtered = append(filtered, "HOME="+home, "XDG_CONFIG_HOME="+configRoot)
	if runtime.GOOS == "windows" {
		// Windows 的 os.UserConfigDir 优先读取 APPDATA，不能继承 runner 的用户目录。
		volume := filepath.VolumeName(home)
		filtered = append(filtered,
			"APPDATA="+configRoot,
			"LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"),
			"USERPROFILE="+home,
			"HOMEDRIVE="+volume,
			"HOMEPATH="+strings.TrimPrefix(home, volume),
		)
	}
	return isolatedProcessEnv{values: filtered, home: home, configRoot: configRoot}
}

func isIsolatedEnvKey(name string) bool {
	for _, key := range []string{
		"HOME", "XDG_CONFIG_HOME", "APPDATA", "LOCALAPPDATA", "USERPROFILE", "HOMEDRIVE", "HOMEPATH",
		"DOWNLOAD_PATH", "FILENAME_TEMPLATE", "https_proxy", "HTTPS_PROXY", "PIXIV_REFRESH_TOKEN",
		"PIXIV_LOG_LEVEL", "PIXIV_LOG_FORMAT",
	} {
		if strings.EqualFold(name, key) {
			return true
		}
	}
	return false
}

func testCommandContext(t *testing.T) context.Context {
	t.Helper()

	if deadline, ok := t.Deadline(); ok {
		ctx, cancel := context.WithDeadline(context.Background(), deadline.Add(-time.Second))
		t.Cleanup(cancel)
		return ctx
	}
	return context.Background()
}
