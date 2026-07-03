package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv"
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

	configPath := exec.Command(binaryPath, "config", "path")
	configPath.Dir = repoRoot
	configPath.Env = env.values
	out, err := configPath.CombinedOutput()
	if err != nil {
		t.Fatalf("pixiv config path failed: %v\n%s", err, string(out))
	}
	gotConfigPath := strings.TrimSpace(string(out))
	if !strings.HasPrefix(gotConfigPath, env.configRoot) && !strings.HasPrefix(gotConfigPath, env.home) {
		t.Fatalf("config path escaped isolated config roots:\n%s", string(out))
	}
	if !strings.HasSuffix(gotConfigPath, filepath.Join("pixiv", "config.toml")) {
		t.Fatalf("config path did not point at pixiv/config.toml:\n%s", string(out))
	}

	configGet := exec.Command(binaryPath, "config", "get", "download_path")
	configGet.Dir = repoRoot
	configGet.Env = env.values
	out, err = configGet.CombinedOutput()
	if err != nil {
		t.Fatalf("pixiv config get download_path failed: %v\n%s", err, string(out))
	}
	if strings.TrimSpace(string(out)) != "./downloads" {
		t.Fatalf("download_path default changed:\n%s", string(out))
	}

	mcpHelp := exec.Command(binaryPath, "mcp", "--help")
	mcpHelp.Dir = repoRoot
	mcpHelp.Env = env.values
	out, err = mcpHelp.CombinedOutput()
	if err != nil {
		t.Fatalf("pixiv mcp --help failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "Run the MCP stdio server") {
		t.Fatalf("mcp help output did not describe stdio server:\n%s", string(out))
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
	var searchResult pixiv.IllustList
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
	var rankingResult pixiv.IllustList
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

	binaryPath := filepath.Join(t.TempDir(), "pixiv")
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
	filtered := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		switch {
		case strings.HasPrefix(entry, "HOME="),
			strings.HasPrefix(entry, "XDG_CONFIG_HOME="),
			strings.HasPrefix(entry, "DOWNLOAD_PATH="),
			strings.HasPrefix(entry, "FILENAME_TEMPLATE="),
			strings.HasPrefix(entry, "https_proxy="),
			strings.HasPrefix(entry, "HTTPS_PROXY="),
			strings.HasPrefix(entry, "PIXIV_REFRESH_TOKEN="):
			continue
		default:
			filtered = append(filtered, entry)
		}
	}
	filtered = append(filtered, "HOME="+home, "XDG_CONFIG_HOME="+configRoot)
	return isolatedProcessEnv{values: filtered, home: home, configRoot: configRoot}
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
