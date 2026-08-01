package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// authenticatedR18RegressionCanaryIDs 刻意只由显式环境变量提供作品 ID，避免
// 测试源码把可能失效、删除或变更分级的真实作品当作固定实现细节。
type authenticatedR18RegressionCanaryIDs struct {
	sfw    int64
	r18    int64
	ugoira int64
}

func authenticatedR18RegressionCanaryIDsFromEnvironment() (authenticatedR18RegressionCanaryIDs, error) {
	values := []struct {
		name string
		set  func(*authenticatedR18RegressionCanaryIDs, int64)
	}{
		{"PIXIV_E2E_SFW_ILLUST_ID", func(ids *authenticatedR18RegressionCanaryIDs, value int64) { ids.sfw = value }},
		{"PIXIV_E2E_R18_ILLUST_ID", func(ids *authenticatedR18RegressionCanaryIDs, value int64) { ids.r18 = value }},
		{"PIXIV_E2E_R18_UGOIRA_ID", func(ids *authenticatedR18RegressionCanaryIDs, value int64) { ids.ugoira = value }},
	}
	var ids authenticatedR18RegressionCanaryIDs
	for _, value := range values {
		raw := strings.TrimSpace(os.Getenv(value.name))
		if raw == "" {
			return authenticatedR18RegressionCanaryIDs{}, fmt.Errorf("%s is required when PIXIV_E2E_REAL_API=1", value.name)
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return authenticatedR18RegressionCanaryIDs{}, fmt.Errorf("%s must be a positive illustration ID", value.name)
		}
		value.set(&ids, id)
	}
	return ids, nil
}

func requireAuthenticatedR18RegressionCanary(t *testing.T) (authenticatedCanaryAuth, authenticatedR18RegressionCanaryIDs) {
	t.Helper()
	if os.Getenv("PIXIV_E2E_REAL_API") != "1" {
		t.Skip("set PIXIV_E2E_REAL_API=1 to run authenticated R18 regression canary")
	}
	auth, skip, err := resolveAuthenticatedCanaryAuth(os.Getenv("PIXIV_E2E_REFRESH_TOKEN"), os.Getenv("PIXIV_E2E_USE_LOCAL_AUTH"))
	if err != nil {
		t.Fatalf("invalid authenticated R18 regression canary credential mode: %v", err)
	}
	if skip {
		t.Skip("set PIXIV_E2E_REFRESH_TOKEN or PIXIV_E2E_USE_LOCAL_AUTH=1 with PIXIV_E2E_REAL_API=1 to run authenticated R18 regression canary")
	}
	ids, err := authenticatedR18RegressionCanaryIDsFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	return auth, ids
}

func requireAuthenticatedCanaryDetailAndPages(t *testing.T, client *pixiv.Client, id int64, wantR18 bool) {
	t.Helper()
	detail, err := client.IllustDetail(testCommandContext(t), id)
	if err != nil {
		t.Fatalf("authenticated detail %d failed: %v", id, err)
	}
	if detail.Illust.ID != id {
		t.Fatalf("authenticated detail returned illustration %d, want %d", detail.Illust.ID, id)
	}
	if wantR18 && detail.Illust.XRestrict == 0 {
		t.Fatalf("authenticated detail %d unexpectedly has x_restrict=0", id)
	}
	if !wantR18 && detail.Illust.XRestrict != 0 {
		t.Fatalf("authenticated SFW detail %d has x_restrict=%d", id, detail.Illust.XRestrict)
	}
	pages, err := client.IllustPages(testCommandContext(t), id)
	if err != nil {
		t.Fatalf("authenticated pages %d failed: %v", id, err)
	}
	if len(pages) == 0 {
		t.Fatalf("authenticated pages %d returned no downloadable pages", id)
	}
	if detail.Illust.PageCount > 0 && len(pages) != detail.Illust.PageCount {
		t.Fatalf("authenticated pages %d count=%d, want detail page_count=%d", id, len(pages), detail.Illust.PageCount)
	}
}

var authenticatedR18RegressionRankingModes = []pixiv.RankingMode{
	pixiv.RankingModeDay,
	pixiv.RankingModeDayMale,
	pixiv.RankingModeDayFemale,
	pixiv.RankingModeWeek,
	pixiv.RankingModeWeekOriginal,
	pixiv.RankingModeWeekRookie,
	pixiv.RankingModeMonth,
	pixiv.RankingModeDayManga,
	pixiv.RankingModeWeekManga,
	pixiv.RankingModeMonthManga,
	pixiv.RankingModeWeekRookieManga,
	pixiv.RankingModeDayR18,
	pixiv.RankingModeDayMaleR18,
	pixiv.RankingModeDayFemaleR18,
	pixiv.RankingModeWeekR18,
	pixiv.RankingModeWeekR18G,
}

// TestPixivSDKAuthenticatedR18RegressionCanary 使用当前源码的 public SDK 验收：
// 认证态不得触发匿名 Web 作品读取，且全部 App 排行榜模式均可访问。
func TestPixivSDKAuthenticatedR18RegressionCanary(t *testing.T) {
	if os.Getenv("PIXIV_E2E_SDK") != "1" {
		t.Skip("set PIXIV_E2E_SDK=1 as well as PIXIV_E2E_REAL_API=1 to run the optional SDK canary")
	}
	auth, ids := requireAuthenticatedR18RegressionCanary(t)
	if err := validateAuthenticatedSDKCanaryBinary(os.Getenv("PIXIV_E2E_BINARY")); err != nil {
		t.Fatal(err)
	}
	proxy := firstNonEmpty(os.Getenv("PIXIV_E2E_PROXY"), os.Getenv("PIXIV_WEB_API_PROXY"))
	options, err := authenticatedCanarySDKOptions(t, auth, proxy)
	if err != nil {
		t.Fatalf("prepare authenticated R18 SDK canary: %v", err)
	}
	client, err := openAuthenticatedCanarySnapshot(testCommandContext(t), options)
	if err != nil {
		t.Fatalf("open authenticated R18 SDK snapshot: %v", err)
	}

	requireAuthenticatedCanaryDetailAndPages(t, client, ids.sfw, false)
	requireAuthenticatedCanaryDetailAndPages(t, client, ids.r18, true)
	for _, mode := range authenticatedR18RegressionRankingModes {
		t.Run(string(mode), func(t *testing.T) {
			result, err := client.IllustRanking(testCommandContext(t), pixiv.IllustRankingRequest{Mode: mode})
			if err != nil {
				t.Fatalf("authenticated ranking %q failed: %v", mode, err)
			}
			if len(result.Illusts) == 0 {
				// 上游可合法发布空榜（例如当前周没有 R18G 条目）；canary 验证的是
				// 16 个 mode 的 App 可达性和 wire 路由，不能把有效空列表误判为故障。
				t.Logf("authenticated ranking %q returned an empty upstream batch", mode)
			}
		})
	}

	ugoira, err := client.UgoiraMetadata(testCommandContext(t), ids.ugoira)
	if err != nil {
		t.Fatalf("authenticated R18 ugoira metadata %d failed: %v", ids.ugoira, err)
	}
	metadata := ugoira.UgoiraMetadata
	if metadata.DownloadURL == "" || metadata.DownloadQuality != pixiv.UgoiraZipQualityMedium {
		t.Fatalf("authenticated R18 ugoira has download_url=%q download_quality=%q, want non-empty medium resource", metadata.DownloadURL, metadata.DownloadQuality)
	}
	if metadata.ZipURLs.Medium != metadata.DownloadURL {
		t.Fatalf("authenticated R18 ugoira medium URL does not match selected download resource")
	}
	if metadata.ZipURLs.Original != "" {
		t.Fatal("authenticated R18 ugoira claimed an unverified original ZIP")
	}
	if len(metadata.Frames) == 0 {
		t.Fatal("authenticated R18 ugoira returned no frames")
	}
}

// TestPixivBinaryAuthenticatedR18RegressionCanary 从当前源码构建 CLI，验证认证态
// R18 详情与动图下载不依赖 Web 补全；下载目录位于 t.TempDir，会自动清理。
func TestPixivBinaryAuthenticatedR18RegressionCanary(t *testing.T) {
	auth, ids := requireAuthenticatedR18RegressionCanary(t)
	if err := validateLocalAuthCanaryBinary(auth.kind, os.Getenv("PIXIV_E2E_BINARY")); err != nil {
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
	provisionExplicitCanaryAuth(t, repoRoot, binaryPath, env, auth)

	for _, test := range []struct {
		name    string
		id      int64
		wantR18 bool
	}{
		{name: "sfw", id: ids.sfw},
		{name: "r18", id: ids.r18, wantR18: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := runPixivCanary(t, repoRoot, binaryPath, env, auth, "detail", strconvFormatInt(test.id), "--json")
			var detail pixiv.IllustDetail
			requireCanaryJSON(t, out, auth, &detail)
			if detail.Illust.ID != test.id {
				t.Fatalf("CLI detail returned illustration %d, want %d", detail.Illust.ID, test.id)
			}
			if test.wantR18 && detail.Illust.XRestrict == 0 {
				t.Fatalf("CLI R18 detail %d unexpectedly has x_restrict=0", test.id)
			}
			if !test.wantR18 && detail.Illust.XRestrict != 0 {
				t.Fatalf("CLI SFW detail %d has x_restrict=%d", test.id, detail.Illust.XRestrict)
			}
			if len(detail.Illust.MetaPages) == 0 {
				t.Fatalf("CLI detail %d returned no normalized downloadable pages", test.id)
			}
		})
	}

	downloadPath := filepath.Join(t.TempDir(), "download")
	_ = runPixivCanary(t, repoRoot, binaryPath, env, auth,
		"download", "--download-path", downloadPath, strconvFormatInt(ids.ugoira))
	requireCanaryAnimation(t, downloadPath, ".gif", []byte("GIF8"))

	apngPath := filepath.Join(t.TempDir(), "download-apng")
	_ = runPixivCanary(t, repoRoot, binaryPath, env, auth,
		"download", "--ugoira-mode", "apng", "--download-path", apngPath, strconvFormatInt(ids.ugoira))
	requireCanaryAnimation(t, apngPath, ".apng", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
}

// requireCanaryAnimation 同时证明 CLI 写出了要求的容器扩展名和文件签名，不把
// 「存在某个普通文件」误当成 GIF/APNG 下载已经成功。
func requireCanaryAnimation(t *testing.T, root, extension string, magic []byte) {
	t.Helper()
	var animation string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() || filepath.Ext(path) != extension {
			return nil
		}
		animation = path
		return filepath.SkipAll
	})
	if err != nil {
		t.Fatal(err)
	}
	if animation == "" {
		t.Fatalf("CLI ugoira download wrote no %s file under %s", extension, root)
	}
	body, err := os.ReadFile(animation)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < len(magic) || string(body[:len(magic)]) != string(magic) {
		t.Fatalf("CLI ugoira output %s does not have the expected %s signature", animation, extension)
	}
}

func requireNonemptyCanaryDownloads(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() == 0 {
			return fmt.Errorf("downloaded file %s is empty", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
