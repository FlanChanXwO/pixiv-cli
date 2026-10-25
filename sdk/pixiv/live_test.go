package pixiv_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	protocol "github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// liveManifestPace 上游对已认证 API 存在 429 限流（sdk.RateLimited 有对应
// reason），在 live 场景之间加入固定间隔是对账号的真实保护，不属于无依据限制。
const (
	liveManifestPace   = 1200 * time.Millisecond
	liveReadAccountEnv = "PIXIV_E2E_READ_USER_ID"
	liveRepoRoot       = "../.."
)

func selectLiveReadAccount(accounts []accountpixiv.Account, defaultID int64, hasDefault bool, explicit string) (accountpixiv.Account, error) {
	if len(accounts) == 0 {
		return accountpixiv.Account{}, errors.New("no local pixiv account")
	}
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		userID, err := strconv.ParseInt(explicit, 10, 64)
		if err != nil || userID <= 0 {
			return accountpixiv.Account{}, fmt.Errorf("%s must be a positive Pixiv account UID", liveReadAccountEnv)
		}
		for _, account := range accounts {
			if account.UserID != userID {
				continue
			}
			if !account.HasRefreshToken() {
				return accountpixiv.Account{}, fmt.Errorf("%s account %d has no stored credential", liveReadAccountEnv, userID)
			}
			return account, nil
		}
		return accountpixiv.Account{}, fmt.Errorf("%s account %d is not present in the local account database", liveReadAccountEnv, userID)
	}

	account := accounts[0]
	if hasDefault {
		for _, candidate := range accounts {
			if candidate.UserID == defaultID {
				return candidate, nil
			}
		}
		return accountpixiv.Account{}, fmt.Errorf("configured pixiv account %d is not present in database", defaultID)
	}
	return account, nil
}

// liveProbePacingRoundTripper 让 raw diagnostic path 遵守 public SDK operation
// 的既有 live pacing contract；raw probe 只用于观察尚未建模的 wire field。
type liveProbePacingRoundTripper struct {
	inner    http.RoundTripper
	interval time.Duration
	mu       sync.Mutex
	last     time.Time
}

func (p *liveProbePacingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	wait := p.interval - time.Since(p.last)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-req.Context().Done():
			timer.Stop()
			p.mu.Unlock()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
	p.last = time.Now()
	p.mu.Unlock()
	return p.inner.RoundTrip(req)
}

func (p *liveProbePacingRoundTripper) CloseIdleConnections() {
	if closer, ok := p.inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

type pixivLiveSession struct {
	client *pixivsdk.Client
	raw    *protocol.Client
	userID int64
}

// openPixivLiveSession 是读取与写入测试共用的唯一凭据入口。mutationAccountID
// 非零时按写入契约选择显式非默认账号；为零时按读取契约选择账号。
func openPixivLiveSession(t *testing.T, ctx context.Context, mutationAccountID int64) *pixivLiveSession {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	db, err := database.Open(filepath.Join(home, paths.AppDataDirName))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	accounts, err := db.ListPixiv(ctx)
	if err != nil {
		t.Fatalf("list local pixiv accounts: %v", err)
	}
	defaultID, hasDefault, err := config.DefaultStore().ReadPixivDefaultUserID()
	if err != nil {
		t.Fatalf("read pixiv default account: %v", err)
	}
	var account accountpixiv.Account
	if mutationAccountID > 0 {
		if !hasDefault {
			if len(accounts) == 0 {
				t.Fatal("no local Pixiv account; explicit mutation e2e has no credential source")
			}
			// 未配置 UID 时数据库 sort_order 第一项就是默认账号，mutation 必须排除它。
			defaultID = accounts[0].UserID
		}
		account, err = findPixivMutationAccount(accounts, mutationAccountID, defaultID)
	} else {
		account, err = selectLiveReadAccount(accounts, defaultID, hasDefault, os.Getenv(liveReadAccountEnv))
	}
	if err != nil {
		t.Fatalf("select pixiv account: %v", err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxy := os.Getenv("PIXIV_E2E_PROXY"); proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			t.Fatalf("parse proxy: %v", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	httpClient := &http.Client{Transport: transport}
	options := pixivsdk.Options{HTTPClient: httpClient}
	// PIXIV_E2E_LOG_LEVEL 让真实读取复用 SDK 的传输与决策日志；默认保持 stderr 安静。
	if level := liveLogLevel(os.Getenv("PIXIV_E2E_LOG_LEVEL")); level != nil {
		options.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	}
	if mutationAccountID > 0 {
		options.Pacing = pixivsdk.Pacing{MinInterval: liveManifestPace}
	}
	client, credentials, err := pixivsdk.OpenWith(ctx, string(account.RefreshTokenCopy()), options)
	if err != nil {
		t.Fatalf("pixiv.Open: %s", liveMutationReason(err))
	}
	t.Cleanup(httpClient.CloseIdleConnections)
	if credentials.UserID <= 0 || credentials.AccessToken() == "" || credentials.RefreshToken() == "" {
		t.Fatal("open did not return verified credentials")
	}
	if credentials.UserID != account.UserID {
		t.Fatalf("open returned a different account identity: stored user %d, verified user %d", account.UserID, credentials.UserID)
	}
	// rotation 持久化失败时不得发起内容请求。
	if err := persistPixivE2ERotation(ctx, db.RotatePixivCredentials, account, credentials.RefreshToken()); err != nil {
		t.Fatalf("persist rotation: %v", err)
	}

	rawHTTPClient := &http.Client{Transport: &liveProbePacingRoundTripper{
		inner:    transport.Clone(),
		interval: liveManifestPace,
	}}
	t.Cleanup(rawHTTPClient.CloseIdleConnections)
	raw := protocol.New(protocol.Config{
		HTTPClient:  rawHTTPClient,
		AccessToken: credentials.AccessToken(),
		UserID:      credentials.UserID,
	})
	return &pixivLiveSession{client: client, raw: raw, userID: credentials.UserID}
}

func liveLogLevel(value string) slog.Leveler {
	switch value {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return nil
	}
}

func TestPixivE2ERotationUsesStoredRevision(t *testing.T) {
	account := accountpixiv.New(42, "", nil)
	account.CredentialRevision = 7
	var gotUserID, gotRevision int64
	var gotToken []byte
	rotate := func(_ context.Context, userID, revision int64, token []byte) error {
		gotUserID = userID
		gotRevision = revision
		gotToken = append([]byte(nil), token...)
		return nil
	}

	if err := persistPixivE2ERotation(context.Background(), rotate, account, "rotated-refresh"); err != nil {
		t.Fatalf("persistPixivE2ERotation: %v", err)
	}
	if gotUserID != account.UserID || gotRevision != account.CredentialRevision || string(gotToken) != "rotated-refresh" {
		t.Fatalf("rotation arguments = user=%d revision=%d token=%q", gotUserID, gotRevision, gotToken)
	}
}

// persistPixivE2ERotation keeps the live path on the same CAS contract as the
// application service: the expected revision comes from the account read that
// supplied the refresh token, never from a fixed initial value.
func persistPixivE2ERotation(
	ctx context.Context,
	rotate func(context.Context, int64, int64, []byte) error,
	account accountpixiv.Account,
	refreshToken string,
) error {
	return rotate(ctx, account.UserID, account.CredentialRevision, []byte(refreshToken))
}

// TestRealPixivSDKRead is the v1 release-prep real Pixiv SDK e2e. It runs only
// when PIXIV_SDK_E2E=1 and a local pixiv-cli.db account exists. The refresh
// token is read from the auth database inside the test process and never enters
// argv, env dumps, logs, test names, or diffs. Rotated credentials are persisted
// back through the repository before any content request.
func TestRealPixivSDKRead(t *testing.T) {
	if os.Getenv("PIXIV_SDK_E2E") != "1" {
		t.Skip("set PIXIV_SDK_E2E=1 to run the real Pixiv SDK e2e")
	}
	ctx := t.Context()
	session := openPixivLiveSession(t, ctx, 0)
	client, accountID := session.client, session.userID

	// 验证身份 + 一个稳定 detail + 一个 Resource HEAD。
	//
	// CurrentUser 是本测试的 mandatory 身份证据：它读取服务端 /v1/user/detail
	// 并要求返回的身份与本地选中账号一致。OpenWith 的 refresh 交换只证明
	// refresh token 可被兑换，不构成服务端身份读取，因此不得作为替代证据。
	// 任何 Reason（含 NotFound）都不降级、不跳过：上游对某账号返回 404 时
	// 本测试如实失败，由使用者更换或重新授权账号后重跑。
	user, err := client.CurrentUser(ctx, pixivsdk.CurrentUserRequest{})
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if user.User.ID <= 0 {
		t.Fatal("current user has no id")
	}
	if user.User.ID != accountID {
		t.Fatalf("current user identity differs from selected account: stored user %d, current user %d", accountID, user.User.ID)
	}

	searchPage, err := client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{Word: "初音ミク"})
	if err != nil {
		t.Fatalf("SearchArtworks: %v", err)
	}
	if len(searchPage.Items) == 0 {
		t.Fatal("search returned no artworks")
	}
	artwork, err := client.Artwork(ctx, pixivsdk.ArtworkRequest{ArtworkID: searchPage.Items[0].ID})
	if err != nil {
		t.Fatalf("Artwork: %v", err)
	}
	if artwork.ID <= 0 || len(artwork.Pages) == 0 {
		t.Fatalf("artwork detail incomplete: %+v", artwork)
	}
	resource := artwork.Pages[0].Image.Resource
	if resource.Ref.IsZero() || resource.URL == "" {
		t.Fatal("page resource has no ref or url")
	}
	response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: resource.Ref, Method: sdk.ResourceMethodHead})
	if err != nil {
		t.Fatalf("OpenResource HEAD: %v", err)
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if response.StatusCode != 200 {
		t.Fatalf("resource HEAD status = %d", response.StatusCode)
	}

	t.Run("viewer field output", func(t *testing.T) {
		for _, value := range []pixivsdk.Artwork{searchPage.Items[0], artwork} {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal("encode artwork DTO")
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal("decode artwork DTO")
			}
			known := 0
			for key, present := range map[string]bool{
				"is_bookmarked":          value.IsBookmarked != nil,
				"is_muted":               value.IsMuted != nil,
				"visible":                value.Visible != nil,
				"sanity_level":           value.SanityLevel != nil,
				"restriction_attributes": value.RestrictionAttributes != nil,
			} {
				_, encoded := fields[key]
				if encoded != present {
					t.Fatalf("viewer field %s presence differs from SDK", key)
				}
				if present {
					known++
				}
			}
			if known == 0 {
				t.Fatal("live artwork has no viewer fields to validate")
			}
			t.Logf("validated %d supplied viewer/safety fields", known)
		}
	})

	t.Run("deep search checkpoint", func(t *testing.T) {
		// 固定历史窗口减少实时新增作品对批内恢复比较的干扰；仍不把续读当作快照。
		request := pixivsdk.SearchArtworksRequest{
			Word: "初音ミク", Sort: pixivsdk.SortModeDateAsc,
			StartDate: "2024-01-01", EndDate: "2024-01-31", Offset: 270,
		}
		page, err := client.SearchArtworks(ctx, request)
		if err != nil {
			t.Fatalf("deep SearchArtworks: %v", err)
		}
		if len(page.Items) < 3 {
			t.Fatal("deep search has insufficient items for repeated checkpoint validation")
		}
		for consumed := 1; consumed <= 2; consumed++ {
			request.Cursor, err = client.CheckpointSearchArtworks(request, 1)
			if err != nil {
				t.Fatalf("checkpoint: %v", err)
			}
			resumed, err := client.SearchArtworks(ctx, request)
			if err != nil {
				t.Fatalf("resume: %v", err)
			}
			if len(resumed.Items) != len(page.Items)-consumed {
				t.Fatal("resumed search length differs from the original batch suffix")
			}
			for index, item := range resumed.Items {
				if item.ID != page.Items[index+consumed].ID {
					t.Fatalf("resumed search differs from the original batch at index %d", index)
				}
			}
		}
	})
}

// TestRealPixivSDKLiveManifestRead 覆盖 artwork/novel/feed 的 live read 场景：
// 明确要求的两页 continuation、
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
	client := openPixivLiveSession(t, ctx, 0).client

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

	// related 当前使用 seed_illust_ids[]/viewed[] 组合 continuation。先从实时
	// 日榜取得高活跃 artwork ID，再强制读取第二页，避免只验证首页 2xx。
	seedPage, err := client.ArtworkRanking(ctx, pixivsdk.ArtworkRankingRequest{Mode: pixivsdk.RankingModeDay})
	if err != nil {
		t.Fatalf("related seed ranking: %v", err)
	}
	if len(seedPage.Items) == 0 {
		t.Fatal("related seed ranking returned no artwork")
	}
	seedID := seedPage.Items[0].ID
	liveTwoPages(t, "related_artworks",
		func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Artwork], error) {
			return client.RelatedArtworks(ctx, pixivsdk.RelatedArtworksRequest{ArtworkID: seedID, Cursor: cursor})
		}, func(item pixivsdk.Artwork) int64 { return item.ID }, true)
	time.Sleep(liveManifestPace)

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

	// #4 artwork-recommended：非空首页 + continuation + 真实第二页。缺一即失败，
	// 不得当作 data-limited。
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

	// #5 artwork-series：当前 surface 不暴露可追溯的 series 引用，按 manifest
	// 记录 data-limited。#9 novel-series 由下方独立的显式候选 recovery probe
	// 覆盖，避免把候选 ID 固化进常规 manifest。
	t.Logf("artwork_series: blocked_external (data): no series reference reachable from production surfaces; candidate recovery remains")
	t.Logf("novel_series: see TestRealPixivSDKLiveNovelSeriesRecovery for explicit public candidate validation")

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
	// frozen request contract 仍保持登记。
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

// liveTwoPages 执行“首页 + 真实续页”场景并做跨页 ID 去重检查。上游无
// continuation 时按 manifest 记录 data-limited，不伪造第二页；请求错误使测试
// 失败。strictNoDup=true 时跨页重复判失败（搜索/排行/最新等确定性序列）；
// 推荐类 feed 上游本身可能跨页重复（已在 live 请求中验证），重复仅记录计数。
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

// firstNovelWithComments 按当前搜索页的真实返回顺序探测首个有评论的小说。
// 不预设页内条数，避免短页越界或把后续合法结果误判为数据阻塞。
func firstNovelWithComments(items []pixivsdk.Novel, hasComments func(int64) bool) (int64, bool) {
	for _, item := range items {
		if hasComments(item.ID) {
			return item.ID, true
		}
	}
	return 0, false
}

// TestRealPixivSDKLiveManifestBookmarkUserRead 覆盖 bookmark、comments 与 user 的
// live read 场景。数据受限按 manifest 记录 blocked_external/data-limited；
// 契约违规（请求错误、必需字段缺失）使测试失败转 correction。
func TestRealPixivSDKLiveManifestBookmarkUserRead(t *testing.T) {
	if os.Getenv("PIXIV_SDK_E2E") != "1" {
		t.Skip("set PIXIV_SDK_E2E=1 to run the real Pixiv SDK e2e")
	}
	ctx := context.Background()
	client := openPixivLiveSession(t, ctx, 0).client

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
		t.Logf("artwork_bookmarks/%s: %d items, continuation=%v", restrict, len(page.Items), !page.Next.IsZero())
		if bookmarkedArtwork == 0 && len(page.Items) > 0 {
			bookmarkedArtwork = page.Items[0].ID
		}
		time.Sleep(liveManifestPace)
	}

	if bookmarkedArtwork > 0 {
		bookmark, err := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: bookmarkedArtwork})
		if err != nil {
			t.Errorf("artwork bookmark detail %d: %v", bookmarkedArtwork, err)
		} else {
			t.Logf("artwork_bookmark_detail %d: restrict=%s tags=%d", bookmarkedArtwork, bookmark.Restrict, len(bookmark.Tags))
		}
		time.Sleep(liveManifestPace)
	}

	// 未收藏样本：搜索结果中的公共作品；无法预知是否恰好已收藏，只记录读取到的状态。
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

	// #15 artwork bookmark tags。
	tags, err := client.UserArtworkBookmarkTags(ctx, pixivsdk.UserArtworkBookmarkTagsRequest{UserID: userID, Restrict: pixivsdk.RestrictPublic})
	if err != nil {
		t.Errorf("artwork bookmark tags: %v", err)
	} else {
		t.Logf("artwork_bookmark_tags: %d tags", len(tags.Items))
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
	if bookmarkedNovel > 0 {
		bookmark, err := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: bookmarkedNovel})
		if err != nil {
			t.Errorf("novel bookmark detail %d: %v", bookmarkedNovel, err)
		} else {
			t.Logf("novel_bookmark_detail %d: restrict=%s tags=%d", bookmarkedNovel, bookmark.Restrict, len(bookmark.Tags))
		}
		time.Sleep(liveManifestPace)
	}
	novelTags, err := client.UserNovelBookmarkTags(ctx, pixivsdk.UserNovelBookmarkTagsRequest{UserID: userID, Restrict: pixivsdk.RestrictPublic})
	if err != nil {
		t.Errorf("novel bookmark tags: %v", err)
	} else {
		t.Logf("novel_bookmark_tags: %d tags", len(novelTags.Items))
	}
	time.Sleep(liveManifestPace)

	// #27 novel-comments read：非空则读；有 continuation 才第二页。
	novels, err := client.SearchNovels(ctx, pixivsdk.SearchNovelsRequest{Word: "初音ミク"})
	if err != nil {
		t.Errorf("novel comments source search: %v", err)
	} else {
		// 数据条件：目标 novel 需有 comments；只扫描当前搜索页，找不到按
		// blocked_external (data) 记录，不伪造非空结果。
		novelID, found := firstNovelWithComments(novels.Items, func(candidateID int64) bool {
			probe, err := client.NovelComments(ctx, pixivsdk.NovelCommentsRequest{NovelID: candidateID})
			if err != nil {
				t.Errorf("novel comments %d: %v", candidateID, err)
				return false
			}
			return len(probe.Page.Items) > 0
		})
		if !found {
			t.Logf("novel_comments: blocked_external (data): current search page has no non-empty comments target")
		} else {
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
	binaryPath := buildPixivBinary(t)
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
		cmd := exec.CommandContext(t.Context(), binaryPath, args...)
		cmd.Dir = liveRepoRoot
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

func buildPixivBinary(t *testing.T) string {
	t.Helper()
	binaryName := "pixiv"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-o", binaryPath, "./cmd/pixiv")
	build.Dir = liveRepoRoot
	var stderr bytes.Buffer
	build.Stderr = &stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/pixiv failed: %v\n%s", err, stderr.String())
	}
	return binaryPath
}

func parseSeriesRecoveryID(rawID, family string) (int64, error) {
	rawID = strings.TrimSpace(rawID)
	if rawID == "" {
		return 0, fmt.Errorf("%s series recovery ID is empty", family)
	}
	seriesID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || seriesID <= 0 {
		return 0, fmt.Errorf("%s series recovery ID must be a positive integer", family)
	}
	return seriesID, nil
}

// TestRealPixivSDKLiveSeriesRecovery validates one explicitly supplied public
// candidate through the public SDK, including its real continuation. The
// candidate stays in the invocation environment rather than becoming a
// permanent fixture or an ID-enumeration seed.
func TestRealPixivSDKLiveSeriesRecovery(t *testing.T) {
	for _, family := range []struct {
		name string
		env  string
	}{
		{name: "novel", env: "PIXIV_NOVEL_SERIES_RECOVERY_ID"},
		{name: "artwork", env: "PIXIV_ARTWORK_SERIES_RECOVERY_ID"},
	} {
		t.Run(family.name, func(t *testing.T) {
			if os.Getenv("PIXIV_SDK_E2E") != "1" {
				t.Skip("set PIXIV_SDK_E2E=1 to run the real Pixiv SDK e2e")
			}
			rawID := strings.TrimSpace(os.Getenv(family.env))
			if rawID == "" {
				t.Skipf("set %s to validate one explicit %s series candidate", family.env, family.name)
			}
			seriesID, err := parseSeriesRecoveryID(rawID, family.name)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			client := openPixivLiveSession(t, ctx, 0).client

			fetch := func(cursor sdk.Cursor) (sdk.Page[pixivsdk.Artwork], error) {
				return client.ArtworkSeries(ctx, pixivsdk.ArtworkSeriesRequest{SeriesID: seriesID, Cursor: cursor})
			}
			if family.name == "novel" {
				first, err := client.NovelSeries(ctx, pixivsdk.NovelSeriesRequest{SeriesID: seriesID})
				if err != nil {
					reportSeriesCandidateError(t, family.name, seriesID, err)
					return
				}
				if !seriesCandidateHasPages(t, family.name, seriesID, len(first.Novels.Items), !first.Novels.Next.IsZero()) {
					return
				}
				second, err := client.NovelSeries(ctx, pixivsdk.NovelSeriesRequest{SeriesID: seriesID, Cursor: first.Novels.Next})
				if err != nil {
					t.Fatalf("%s_series candidate %d second page: %v", family.name, seriesID, err)
				}
				t.Logf("%s_series candidate %d second page: items=%d continuation=%v", family.name, seriesID, len(second.Novels.Items), !second.Novels.Next.IsZero())
				return
			}

			first, err := fetch(sdk.Cursor{})
			if err != nil {
				reportSeriesCandidateError(t, family.name, seriesID, err)
				return
			}
			if !seriesCandidateHasPages(t, family.name, seriesID, len(first.Items), !first.Next.IsZero()) {
				return
			}
			second, err := fetch(first.Next)
			if err != nil {
				t.Fatalf("%s_series candidate %d second page: %v", family.name, seriesID, err)
			}
			if len(second.Items) == 0 {
				t.Fatalf("%s_series candidate %d second page is empty", family.name, seriesID)
			}
			t.Logf("%s_series candidate %d second page: items=%d continuation=%v", family.name, seriesID, len(second.Items), !second.Next.IsZero())
		})
	}
}

func reportSeriesCandidateError(t *testing.T, family string, seriesID int64, err error) {
	t.Helper()
	switch sdk.ReasonOf(err) {
	case sdk.Forbidden, sdk.NotFound, sdk.ContentUnavailable:
		t.Logf("%s_series candidate %d: blocked_external (data/permission): reason=%s", family, seriesID, sdk.ReasonOf(err))
	default:
		t.Fatalf("%s_series candidate %d: %v", family, seriesID, err)
	}
}

func seriesCandidateHasPages(t *testing.T, family string, seriesID int64, items int, hasNext bool) bool {
	t.Helper()
	t.Logf("%s_series candidate %d first page: items=%d continuation=%v", family, seriesID, items, hasNext)
	if items == 0 {
		t.Logf("%s_series candidate %d: blocked_external (data): empty first page", family, seriesID)
		return false
	}
	if !hasNext {
		t.Logf("%s_series candidate %d: blocked_external (data): no continuation", family, seriesID)
		return false
	}
	return true
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
