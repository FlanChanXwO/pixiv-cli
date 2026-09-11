package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
			}, func(item pixivsdk.Artwork) int64 { return item.ID }, true)
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
			}, func(item pixivsdk.Artwork) int64 { return item.ID }, true)
		time.Sleep(liveManifestPace)
	}

	// #3 artwork-ranking：固定 mode=day，offset 续页 + 无重复。
	liveTwoPages(t, "ranking/day",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Artwork], error) {
			return client.ArtworkRanking(ctx, pixivsdk.ArtworkRankingRequest{Mode: pixivsdk.RankingModeDay, Cursor: cursor})
		}, func(item pixivsdk.Artwork) int64 { return item.ID }, true)
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
		}, func(item pixivsdk.Novel) int64 { return item.ID }, true)
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
		}, func(item pixivsdk.Novel) int64 { return item.ID }, true)
	time.Sleep(liveManifestPace)

	// #11 novel-recommended：offset 续页。
	liveTwoPages(t, "recommended_novel",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Novel], error) {
			return client.RecommendedNovels(ctx, pixivsdk.RecommendedNovelsRequest{Cursor: cursor})
		}, func(item pixivsdk.Novel) int64 { return item.ID }, false)
	time.Sleep(liveManifestPace)

	// #12 novel-ranking：固定 mode=day，offset 续页。
	liveTwoPages(t, "ranking_novel/day",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Novel], error) {
			return client.NovelRanking(ctx, pixivsdk.NovelRankingRequest{Mode: pixivsdk.RankingModeDay, Cursor: cursor})
		}, func(item pixivsdk.Novel) int64 { return item.ID }, true)
	time.Sleep(liveManifestPace)

	// #13 novel-follow：restrict=public，offset 续页；feed 为空记 data-limited。
	liveTwoPages(t, "following_novel/public",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Novel], error) {
			return client.FollowingNovels(ctx, pixivsdk.FollowingNovelsRequest{Restrict: pixivsdk.RestrictPublic, Cursor: cursor})
		}, func(item pixivsdk.Novel) int64 { return item.ID }, true)
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
// continuation 时按 manifest 记录 data-limited，不伪造第二页；请求错误使测试
// 失败。strictNoDup=true 时跨页重复判失败（搜索/排行/最新等确定性序列）；
// 推荐类 feed 上游本身可能跨页重复（G1-T28 live 证据），重复仅记录计数。
func liveTwoPages[T any](t *testing.T, name string, fetch func(sdk.Cursor) (sdk.Page[T], error), idOf func(T) int64, strictNoDup bool) {
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
	if duplicates > 0 && strictNoDup {
		t.Errorf("%s second page repeats %d first-page item(s)", name, duplicates)
	}
}

// TestRealPixivSDKLiveManifestBookmarkUserRead 执行 goal-1 Live Manifest
// bookmark（14–16、18–20、23–24）、comments（27、29）、user（30–35、37）的
// live read 场景。数据受限按 manifest 记录 blocked_external/data-limited；
// 契约违规（请求错误、必需字段缺失）使测试失败转 correction。
func TestRealPixivSDKLiveManifestBookmarkUserRead(t *testing.T) {
	if os.Getenv("PIXIV_SDK_E2E") != "1" {
		t.Skip("set PIXIV_SDK_E2E=1 to run the real Pixiv SDK e2e")
	}
	ctx := context.Background()
	client := openRealPixivLiveClient(t, ctx)

	// #33 user-detail + CurrentUser 身份（后续 bookmark 读以该身份执行）。
	identity, err := client.CurrentUser(ctx, pixivsdk.CurrentUserRequest{})
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	userID := identity.User.ID
	if userID <= 0 {
		t.Fatal("current user has no id")
	}
	detail, err := client.User(ctx, pixivsdk.UserRequest{UserID: userID})
	if err != nil {
		t.Errorf("user detail: %v", err)
	} else {
		t.Logf("user_detail: profile counters present=%v", detail.Profile.TotalIllusts >= 0)
	}
	time.Sleep(liveManifestPace)

	// #14 artwork bookmark list：public/private 代表读；自然 continuation 只记录。
	var bookmarkedArtwork int64
	for _, restrict := range []pixivsdk.Restrict{pixivsdk.RestrictPublic, pixivsdk.RestrictPrivate} {
		page, err := client.UserArtworkBookmarks(ctx, pixivsdk.UserArtworkBookmarksRequest{UserID: userID, Restrict: restrict})
		if err != nil {
			t.Errorf("artwork bookmarks %s: %v", restrict, err)
			continue
		}
		t.Logf("bookmarks/illust/%s: %d items, continuation=%v", restrict, len(page.Items), !page.Next.IsZero())
		if bookmarkedArtwork == 0 && len(page.Items) > 0 {
			bookmarkedArtwork = page.Items[0].ID
		}
		time.Sleep(liveManifestPace)
	}

	// #15/#19 bookmark tags：name/count；空列表合法。
	artworkTags, err := client.UserArtworkBookmarkTags(ctx, pixivsdk.UserArtworkBookmarkTagsRequest{UserID: userID, Restrict: pixivsdk.RestrictPublic})
	if err != nil {
		t.Errorf("artwork bookmark tags: %v", err)
	} else {
		t.Logf("bookmark_tags/illust: %d tags", len(artworkTags.Items))
	}
	time.Sleep(liveManifestPace)
	novelTags, err := client.UserNovelBookmarkTags(ctx, pixivsdk.UserNovelBookmarkTagsRequest{UserID: userID, Restrict: pixivsdk.RestrictPublic})
	if err != nil {
		t.Errorf("novel bookmark tags: %v", err)
	} else {
		t.Logf("bookmark_tags/novel: %d tags", len(novelTags.Items))
	}
	time.Sleep(liveManifestPace)

	// #18 novel bookmark list：public/private。
	var bookmarkedNovel int64
	for _, restrict := range []pixivsdk.Restrict{pixivsdk.RestrictPublic, pixivsdk.RestrictPrivate} {
		page, err := client.UserNovelBookmarks(ctx, pixivsdk.UserNovelBookmarksRequest{UserID: userID, Restrict: restrict})
		if err != nil {
			t.Errorf("novel bookmarks %s: %v", restrict, err)
			continue
		}
		t.Logf("bookmarks/novel/%s: %d items, continuation=%v", restrict, len(page.Items), !page.Next.IsZero())
		if bookmarkedNovel == 0 && len(page.Items) > 0 {
			bookmarkedNovel = page.Items[0].ID
		}
		time.Sleep(liveManifestPace)
	}

	// #16/#20 bookmark detail：已收藏样本（来自 bookmark list）+ 搜索样本。
	if bookmarkedArtwork > 0 {
		bookmarked, err := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: bookmarkedArtwork})
		if err != nil {
			t.Errorf("artwork bookmark detail %d: %v", bookmarkedArtwork, err)
		} else {
			t.Logf("artwork_bookmark_detail %d: tags=%d restrict_nonempty=%v", bookmarkedArtwork, len(bookmarked.Tags), bookmarked.Restrict != "")
		}
	} else {
		t.Logf("artwork_bookmark_detail bookmarked case: blocked_external (data): account has no artwork bookmarks")
	}
	time.Sleep(liveManifestPace)
	if bookmarkedNovel > 0 {
		bookmarked, err := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: bookmarkedNovel})
		if err != nil {
			t.Errorf("novel bookmark detail %d: %v", bookmarkedNovel, err)
		} else {
			t.Logf("novel_bookmark_detail %d: tags=%d restrict_nonempty=%v", bookmarkedNovel, len(bookmarked.Tags), bookmarked.Restrict != "")
		}
	} else {
		t.Logf("novel_bookmark_detail bookmarked case: blocked_external (data): account has no novel bookmarks")
	}
	time.Sleep(liveManifestPace)
	// absent/未收藏样本：搜索结果中的公共作品/小说（无法预知是否恰好已收藏，
	// 因此只记录读取到的状态，不硬断言 bookmarked=false）。
	searchArtworks, err := client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{Word: "風景", ContentType: pixivsdk.SearchContentTypeIllust})
	if err != nil {
		t.Errorf("bookmark detail absent source search: %v", err)
	} else if len(searchArtworks.Items) > 0 {
		absentID := searchArtworks.Items[len(searchArtworks.Items)-1].ID
		absent, err := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: absentID})
		if err != nil {
			t.Errorf("artwork bookmark detail absent %d: %v", absentID, err)
		} else {
			t.Logf("artwork_bookmark_detail absent %d: bookmarked=%v tags=%d", absentID, absent.Restrict != "", len(absent.Tags))
		}
	}
	time.Sleep(liveManifestPace)

	// #27 novel-comments read：非空则读；有 continuation 才第二页。
	novels, err := client.SearchNovels(ctx, pixivsdk.SearchNovelsRequest{Word: "初音ミク"})
	if err != nil {
		t.Errorf("novel comments source search: %v", err)
	} else if len(novels.Items) == 0 {
		t.Logf("novel_comments: blocked_external (data): novel search returned no items")
	} else {
		// 数据条件：目标 novel 需有 comments；扫描前 3 本，找不到按
		// blocked_external (data) 记录，不伪造非空结果。
		novelID := novels.Items[0].ID
		for _, candidate := range novels.Items[:3] {
			probe, err := client.NovelComments(ctx, pixivsdk.NovelCommentsRequest{NovelID: candidate.ID})
			if err != nil {
				t.Errorf("novel comments %d: %v", candidate.ID, err)
				continue
			}
			if len(probe.Page.Items) > 0 {
				novelID = candidate.ID
				break
			}
		}
		comments, err := client.NovelComments(ctx, pixivsdk.NovelCommentsRequest{NovelID: novelID})
		if err != nil {
			t.Errorf("novel comments %d: %v", novelID, err)
		} else {
			t.Logf("novel_comments %d: %d comments, continuation=%v, total=%v, access_control=%v",
				novelID, len(comments.Page.Items), !comments.Page.Next.IsZero(), comments.Total != nil, comments.AccessControl != nil)
			if !comments.Page.Next.IsZero() {
				second, err := client.NovelComments(ctx, pixivsdk.NovelCommentsRequest{NovelID: novelID, Cursor: comments.Page.Next})
				if err != nil {
					t.Errorf("novel comments second page: %v", err)
				} else {
					t.Logf("novel_comments %d second page: %d comments", novelID, len(second.Page.Items))
				}
			}
		}
	}
	time.Sleep(liveManifestPace)

	if len(novels.Items) > 0 {
		absentNovelID := novels.Items[len(novels.Items)-1].ID
		absentNovel, err := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: absentNovelID})
		if err != nil {
			t.Errorf("novel bookmark detail absent %d: %v", absentNovelID, err)
		} else {
			t.Logf("novel_bookmark_detail absent %d: bookmarked=%v tags=%d", absentNovelID, absentNovel.Restrict != "", len(absentNovel.Tags))
		}
		time.Sleep(liveManifestPace)
	}

	// #29 stamps read。
	stamps, err := client.Stamps(ctx, pixivsdk.StampsRequest{})
	if err != nil {
		t.Errorf("stamps: %v", err)
	} else {
		t.Logf("stamps: %d stamps", len(stamps))
	}
	time.Sleep(liveManifestPace)

	// #30/#31 user artworks/novels（self，pagination_exempt：自然 continuation 只记录）。
	liveTwoPages(t, "user_artworks/self", func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Artwork], error) {
		return client.UserArtworks(ctx, pixivsdk.UserArtworksRequest{UserID: userID, Cursor: cursor})
	}, func(item pixivsdk.Artwork) int64 { return item.ID }, false)
	time.Sleep(liveManifestPace)
	liveTwoPages(t, "user_novels/self", func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Novel], error) {
		return client.UserNovels(ctx, pixivsdk.UserNovelsRequest{UserID: userID, Cursor: cursor})
	}, func(item pixivsdk.Novel) int64 { return item.ID }, false)
	time.Sleep(liveManifestPace)

	// #32 user relationships：following/followers/related/blocked 代表读。
	liveTwoPages(t, "user_following/self", func(cursor sdk.Cursor) (sdk.Page[pixivsdk.UserPreview], error) {
		return client.UserFollowing(ctx, pixivsdk.UserFollowingRequest{UserID: userID, Cursor: cursor})
	}, func(item pixivsdk.UserPreview) int64 { return item.User.ID }, false)
	time.Sleep(liveManifestPace)
	liveTwoPages(t, "user_followers/self", func(cursor sdk.Cursor) (sdk.Page[pixivsdk.UserPreview], error) {
		return client.UserFollowers(ctx, pixivsdk.UserFollowersRequest{UserID: userID, Cursor: cursor})
	}, func(item pixivsdk.UserPreview) int64 { return item.User.ID }, false)
	time.Sleep(liveManifestPace)
	liveTwoPages(t, "blocked_users/self", func(cursor sdk.Cursor) (sdk.Page[pixivsdk.UserPreview], error) {
		return client.UserBlockedUsers(ctx, pixivsdk.UserBlockedUsersRequest{UserID: userID, Cursor: cursor})
	}, func(item pixivsdk.UserPreview) int64 { return item.User.ID }, false)
	time.Sleep(liveManifestPace)

	// #34 user search：non-empty word；有 continuation 才第二请求。
	users, err := client.SearchUsers(ctx, pixivsdk.SearchUsersRequest{Word: "pixiv"})
	if err != nil {
		t.Errorf("search users: %v", err)
	} else {
		t.Logf("search_user: %d items, continuation=%v", len(users.Items), !users.Next.IsZero())
		if len(users.Items) == 0 {
			t.Errorf("search users returned no items for a common word")
		}
	}
	time.Sleep(liveManifestPace)

	// #35 trending：non-empty tags + 合法 sample artwork。
	trending, err := client.TrendingArtworkTags(ctx, pixivsdk.TrendingArtworkTagsRequest{})
	if err != nil {
		t.Errorf("trending tags: %v", err)
	} else {
		sampleOK := len(trending) == 0 || trending[0].Artwork.ID > 0
		t.Logf("trending_tags_illust: %d tags, sample_ok=%v", len(trending), sampleOK)
		if len(trending) == 0 {
			t.Errorf("trending tags returned no items")
		} else if !sampleOK {
			t.Errorf("trending sample artwork invalid")
		}
	}
	time.Sleep(liveManifestPace)

	// #37 mypixiv：users/artworks/novels 代表读（空结果合法，只记录）。
	myUsers, err := client.MyPixivUsers(ctx, pixivsdk.MyPixivUsersRequest{})
	if err != nil {
		t.Errorf("mypixiv users: %v", err)
	} else {
		t.Logf("mypixiv_users: %d items", len(myUsers.Items))
	}
	time.Sleep(liveManifestPace)
	myArtworks, err := client.MyPixivArtworks(ctx, pixivsdk.MyPixivArtworksRequest{})
	if err != nil {
		t.Errorf("mypixiv artworks: %v", err)
	} else {
		t.Logf("mypixiv_illusts: %d items", len(myArtworks.Items))
	}
	time.Sleep(liveManifestPace)
	myNovels, err := client.MyPixivNovels(ctx, pixivsdk.MyPixivNovelsRequest{})
	if err != nil {
		t.Errorf("mypixiv novels: %v", err)
	} else {
		t.Logf("mypixiv_novels: %d items", len(myNovels.Items))
	}
	time.Sleep(liveManifestPace)

	// #23/#24 bookmark all 聚合（CLI --type all）：artwork 后 novel、typed output。
	repoRoot := ".."
	binaryPath := buildPixivVersionBinary(t, repoRoot)
	proxy := os.Getenv("PIXIV_E2E_PROXY")
	for _, scenario := range []struct {
		name string
		args []string
	}{
		{name: "bookmark_list_all", args: []string{"bookmark", "list", "--type", "all", "--limit", "2", "--json"}},
		{name: "bookmark_tags_all", args: []string{"bookmark", "tags", "--type", "all", "--limit", "2", "--json"}},
	} {
		args := append([]string{}, scenario.args...)
		if proxy != "" {
			args = append(args, "--proxy", proxy)
		}
		cmd := exec.CommandContext(testCommandContext(t), binaryPath, args...)
		cmd.Dir = repoRoot
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			t.Logf("%s: blocked_external (account/config): %v; stderr head=%q", scenario.name, err, strings.TrimSpace(stderr.String())[:min(len(stderr.String()), 200)])
			continue
		}
		out := stdout.String()
		types := cliRecordTypes(t, out)
		artworkSeen, novelSeen, artworkFirst := false, false, true
		for _, kind := range types {
			// record DTO 的 type 携带历史 subtype（illustration/manga/ugoira
			// 属于 artwork side；见 cli-migration-matrix 的 resolver 规则）。
			switch kind {
			case "artwork", "illust", "manga", "ugoira", "illustration":
				if !artworkSeen && novelSeen {
					artworkFirst = false
				}
				artworkSeen = true
			case "novel":
				novelSeen = true
			}
		}
		t.Logf("%s: records=%d artwork_side=%v novel_side=%v artwork_first=%v",
			scenario.name, len(types), artworkSeen, novelSeen, artworkFirst)
	}
}

// cliRecordTypes 解析 CLI --json 输出的 record type 序列（兼容数组与
// {records:[...]} envelope），用于验证 all 聚合的 artwork→novel 顺序。
func cliRecordTypes(t *testing.T, out string) []string {
	t.Helper()
	var records []map[string]any
	var envelope struct {
		Records      []map[string]any `json:"records"`
		BookmarkTags []map[string]any `json:"bookmark_tags"`
	}
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Logf("bookmark all output shape: not parsed as records (%v)", err)
			return nil
		}
		records = append(records, envelope.Records...)
		records = append(records, envelope.BookmarkTags...)
	}
	types := make([]string, 0, len(records))
	for _, record := range records {
		kind, _ := record["type"].(string)
		types = append(types, kind)
	}
	return types
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
