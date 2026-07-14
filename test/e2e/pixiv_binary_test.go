package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	// 配置路径是 stdout 协议；自动更新 warning 和结构化日志只能在 stderr，不能用
	// CombinedOutput 混合后再判断路径。
	out, _ := runPixivStdout(t, repoRoot, binaryPath, env.values, "config", "path")
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
	if refreshToken := os.Getenv("PIXIV_E2E_REFRESH_TOKEN"); refreshToken != "" {
		env = append(env, "PIXIV_REFRESH_TOKEN="+refreshToken)
	} else {
		env = append(env, "PIXIV_REFRESH_TOKEN=")
	}

	run := exec.CommandContext(testCommandContext(t), binaryPath, "search", "初音ミク")
	run.Dir = repoRoot
	run.Env = env
	out, err := run.CombinedOutput()
	if os.Getenv("PIXIV_E2E_REFRESH_TOKEN") == "" {
		if err != nil {
			t.Fatalf("unauthenticated real web fallback search failed: %v\n%s", err, string(out))
		}
		if !strings.Contains(string(out), "found ") {
			t.Fatalf("unauthenticated real web fallback search did not print result summary:\n%s", string(out))
		}
		return
	}
	if err != nil {
		t.Fatalf("pixiv search against real API failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "found ") {
		t.Fatalf("real API search did not print result summary:\n%s", string(out))
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
