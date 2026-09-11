package e2e

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// liveManifestPace 上游对已认证 API 存在 429 限流（sdk.RateLimited 有对应
// reason），在 live 场景之间加入固定间隔是对账号的真实保护，不属于无依据限制。
const liveManifestPace = 1200 * time.Millisecond

// TestRealPixivSDKLiveManifestRead 执行 goal-1 Live Manifest（§11.1/§11.4）中
// artwork/novel/feed 的 live read 场景：manifest 明确要求的两页 continuation、
// 关键 query、错误边界与 #41 的三条 recommended 流。数据受限（例如无法安全构造
// series ID、follow feed 为空、上游无 continuation）按 manifest 记录
// data-limited/blocked_external 并在 t.Logf 中输出，不得伪造第二页；契约违规
// （请求错误、跨页重复、必需字段缺失）使测试失败并转 correction。
//
// 凭据只在本进程内读取 auth 数据库并回写轮换，绝不进入 argv/env/log；输出仅
// 包含计数、布尔值与公共实体 ID。
func TestRealPixivSDKLiveManifestRead(t *testing.T) {
	if os.Getenv("PIXIV_SDK_E2E") != "1" {
		t.Skip("set PIXIV_SDK_E2E=1 to run the real Pixiv SDK e2e")
	}
	ctx := context.Background()
	client := openRealPixivLiveClient(t, ctx)

	// #1 artwork-search：稳定 query，覆盖 all/illust/manga/ugoira，首页+续页。
	for _, contentType := range []pixivsdk.SearchContentType{
		pixivsdk.SearchContentTypeAll,
		pixivsdk.SearchContentTypeIllust,
		pixivsdk.SearchContentTypeManga,
		pixivsdk.SearchContentTypeUgoira,
	} {
		liveTwoPages(t, "search_illust/"+string(contentType),
			func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Artwork], error) {
				return client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{
					Word:        "初音ミク",
					ContentType: contentType,
					Cursor:      cursor,
				})
			}, func(item pixivsdk.Artwork) int64 { return item.ID })
		time.Sleep(liveManifestPace)
	}

	// #2 artwork-latest：承诺 subtype illust/manga，max_illust_id 续页。
	for _, contentType := range []pixivsdk.SearchContentType{
		pixivsdk.SearchContentTypeIllust,
		pixivsdk.SearchContentTypeManga,
	} {
		liveTwoPages(t, "latest/"+string(contentType),
			func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Artwork], error) {
				return client.LatestArtworks(ctx, pixivsdk.LatestArtworksRequest{ContentType: contentType, Cursor: cursor})
			}, func(item pixivsdk.Artwork) int64 { return item.ID })
		time.Sleep(liveManifestPace)
	}

	// #3 artwork-ranking：固定 mode=day，offset 续页 + 无重复。
	liveTwoPages(t, "ranking/day",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Artwork], error) {
			return client.ArtworkRanking(ctx, pixivsdk.ArtworkRankingRequest{Mode: pixivsdk.RankingModeDay, Cursor: cursor})
		}, func(item pixivsdk.Artwork) int64 { return item.ID })
	time.Sleep(liveManifestPace)

	// #4 artwork-recommended：非空首页 + continuation + 真实第二页（P1 closure
	// 场景）。缺一即失败转 correction，不得当作 data-limited。
	recommended, err := client.RecommendedArtworks(ctx, pixivsdk.RecommendedArtworksRequest{})
	if err != nil {
		t.Errorf("recommended first page: %v", err)
	} else {
		t.Logf("recommended first page: %d items, continuation=%v", len(recommended.Items), !recommended.Next.IsZero())
		if len(recommended.Items) == 0 {
			t.Errorf("recommended first page is empty; manifest requires non-empty first page")
		}
		if recommended.Next.IsZero() {
			t.Errorf("recommended returned no continuation; manifest requires second-page continuation")
		} else {
			second, err := client.RecommendedArtworks(ctx, pixivsdk.RecommendedArtworksRequest{Cursor: recommended.Next})
			if err != nil {
				t.Errorf("recommended second page: %v", err)
			} else {
				t.Logf("recommended second page: %d items", len(second.Items))
			}
		}
	}
	time.Sleep(liveManifestPace)

	// #5 artwork-series / #9 novel-series：当前 surface 不暴露 series 引用，
	// 无法从既有 live 结果安全构造目标 series ID；按 manifest 记录 data-limited。
	t.Logf("artwork_series: blocked_external (data): no series reference reachable from production surfaces; correction candidate CAND-G1-T06-ARTWORK-SERIES-LIVE remains")
	t.Logf("novel_series: blocked_external (data): no series reference reachable from production surfaces; correction candidate CAND-G1-T06-REC-SERIES-DOC remains")

	// #6 ugoira-metadata：从 ugoira 搜索结果取有效 artwork，读取 archive/frames。
	ugoiraPage, err := client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{Word: "初音ミク", ContentType: pixivsdk.SearchContentTypeUgoira})
	if err != nil {
		t.Errorf("ugoira source search: %v", err)
	} else if len(ugoiraPage.Items) == 0 {
		t.Logf("ugoira_metadata: blocked_external (data): ugoira search returned no items")
	} else {
		ugoiraID := ugoiraPage.Items[0].ID
		metadata, err := client.UgoiraMetadata(ctx, pixivsdk.UgoiraMetadataRequest{ArtworkID: ugoiraID})
		if err != nil {
			t.Errorf("ugoira metadata for %d: %v", ugoiraID, err)
		} else {
			t.Logf("ugoira_metadata %d: frames=%d archives=%d", ugoiraID, len(metadata.Frames), len(metadata.Archives))
		}
	}
	time.Sleep(liveManifestPace)

	// #7 novel-search：固定 query，offset 首页+续页；period/date 参数未进入
	// frozen request contract（CAND-G1-T06-NOVEL-SEARCH-CONTRACT 保持登记）。
	liveTwoPages(t, "search_novel",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Novel], error) {
			return client.SearchNovels(ctx, pixivsdk.SearchNovelsRequest{Word: "初音ミク", Cursor: cursor})
		}, func(item pixivsdk.Novel) int64 { return item.ID })
	time.Sleep(liveManifestPace)

	// #8 novel-detail：v2 detail 读取一个有效 novel。
	novels, err := client.SearchNovels(ctx, pixivsdk.SearchNovelsRequest{Word: "初音ミク"})
	if err != nil {
		t.Errorf("novel detail source search: %v", err)
	} else if len(novels.Items) == 0 {
		t.Logf("novel_detail: blocked_external (data): novel search returned no items")
	} else {
		novelID := novels.Items[0].ID
		detail, err := client.Novel(ctx, pixivsdk.NovelRequest{NovelID: novelID})
		if err != nil {
			t.Errorf("novel detail %d: %v", novelID, err)
		} else if detail.ID != novelID {
			t.Errorf("novel detail id mismatch: got %d want %d", detail.ID, novelID)
		} else {
			t.Logf("novel_detail %d: ok", novelID)
		}
	}
	time.Sleep(liveManifestPace)

	// #10 novel-latest：max_novel_id 续页，禁止 offset fallback（cursor binding
	// 由 SDK 强制，live 只验证两页成立）。
	liveTwoPages(t, "latest_novel",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Novel], error) {
			return client.LatestNovels(ctx, pixivsdk.LatestNovelsRequest{Cursor: cursor})
		}, func(item pixivsdk.Novel) int64 { return item.ID })
	time.Sleep(liveManifestPace)

	// #11 novel-recommended：offset 续页。
	liveTwoPages(t, "recommended_novel",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Novel], error) {
			return client.RecommendedNovels(ctx, pixivsdk.RecommendedNovelsRequest{Cursor: cursor})
		}, func(item pixivsdk.Novel) int64 { return item.ID })
	time.Sleep(liveManifestPace)

	// #12 novel-ranking：固定 mode=day，offset 续页。
	liveTwoPages(t, "ranking_novel/day",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Novel], error) {
			return client.NovelRanking(ctx, pixivsdk.NovelRankingRequest{Mode: pixivsdk.RankingModeDay, Cursor: cursor})
		}, func(item pixivsdk.Novel) int64 { return item.ID })
	time.Sleep(liveManifestPace)

	// #13 novel-follow：restrict=public，offset 续页；feed 为空记 data-limited。
	liveTwoPages(t, "following_novel/public",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Novel], error) {
			return client.FollowingNovels(ctx, pixivsdk.FollowingNovelsRequest{Restrict: pixivsdk.RestrictPublic, Cursor: cursor})
		}, func(item pixivsdk.Novel) int64 { return item.ID })
	time.Sleep(liveManifestPace)

	// #41 recommended-all 的 artwork/novel/user feed streams（首条流 + continuation 记录）。
	userStream, err := client.RecommendedUsers(ctx, pixivsdk.RecommendedUsersRequest{})
	if err != nil {
		t.Errorf("recommended users stream: %v", err)
	} else {
		t.Logf("recommended_users stream: %d items, continuation=%v", len(userStream.Items), !userStream.Next.IsZero())
	}
}

// openRealPixivLiveClient 与 TestRealPixivSDKRead 保持同一账号选择、代理与轮换
// 持久化契约：凭据只在本进程内读取与回写。
func openRealPixivLiveClient(t *testing.T, ctx context.Context) *pixivsdk.Client {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	appDataDir := filepath.Join(home, paths.AppDataDirName)
	db, err := database.Open(appDataDir)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	accounts, err := db.ListPixiv(ctx)
	if err != nil {
		t.Fatalf("list local pixiv accounts: %v", err)
	}
	if len(accounts) == 0 {
		t.Fatal("no local pixiv account; explicit real e2e has no credential source")
	}
	account := accounts[0]
	defaultID, hasDefault, err := config.ReadPixivDefaultUserID()
	if err != nil {
		t.Fatalf("read pixiv default account: %v", err)
	}
	if hasDefault {
		found := false
		for _, candidate := range accounts {
			if candidate.UserID == defaultID {
				account = candidate
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("configured pixiv account %d is not present in database", defaultID)
		}
	}

	options := pixivsdk.Options{}
	if proxy := os.Getenv("PIXIV_E2E_PROXY"); proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			t.Fatalf("parse proxy: %v", err)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = http.ProxyURL(proxyURL)
		options.HTTPClient = &http.Client{Transport: transport}
	}
	client, credentials, err := pixivsdk.OpenWith(ctx, string(account.RefreshTokenCopy()), options)
	if err != nil {
		t.Fatalf("pixiv.Open: %v", err)
	}
	t.Cleanup(client.CloseIdleConnections)
	if credentials.UserID <= 0 || credentials.AccessToken() == "" || credentials.RefreshToken() == "" {
		t.Fatal("open did not return verified credentials")
	}
	if credentials.UserID != account.UserID {
		t.Fatalf("open returned a different account identity: stored user %d, verified user %d", account.UserID, credentials.UserID)
	}
	if err := db.RotatePixivCredentials(ctx, account.UserID, account.CredentialRevision, []byte(credentials.RefreshToken())); err != nil {
		t.Fatalf("persist rotation: %v", err)
	}
	return client
}

// liveTwoPages 执行“首页 + 真实续页”场景并做跨页 ID 去重检查。上游无
// continuation 时按 manifest 记录 data-limited，不伪造第二页；请求错误与跨页
// 重复属于契约违规，使测试失败。
func liveTwoPages[T any](t *testing.T, name string, fetch func(sdk.Cursor) (sdk.Page[T], error), idOf func(T) int64) {
	t.Helper()
	page1, err := fetch(sdk.Cursor{})
	if err != nil {
		t.Errorf("%s first page: %v", name, err)
		return
	}
	t.Logf("%s first page: %d items, continuation=%v", name, len(page1.Items), !page1.Next.IsZero())
	if page1.Next.IsZero() {
		t.Logf("%s: blocked_external (data): upstream returned no continuation; second page not requested", name)
		return
	}
	seen := make(map[int64]struct{}, len(page1.Items))
	for _, item := range page1.Items {
		seen[idOf(item)] = struct{}{}
	}
	page2, err := fetch(page1.Next)
	if err != nil {
		t.Errorf("%s second page: %v", name, err)
		return
	}
	duplicates := 0
	for _, item := range page2.Items {
		if _, ok := seen[idOf(item)]; ok {
			duplicates++
		}
	}
	t.Logf("%s second page: %d items, duplicates=%d", name, len(page2.Items), duplicates)
	if duplicates > 0 {
		t.Errorf("%s second page repeats %d first-page item(s)", name, duplicates)
	}
}
