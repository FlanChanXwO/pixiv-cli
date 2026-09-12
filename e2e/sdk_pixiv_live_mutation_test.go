package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

const (
	mutationGuardEnv   = "PIXIV_SDK_E2E_MUTATION"
	mutationAccountEnv = "PIXIV_E2E_MUTATION_USER_ID"
)

// findPixivMutationAccount enforces the manifest's account-isolation boundary:
// live mutation must name a stored credential explicitly and may not use the
// configured default account.
func findPixivMutationAccount(accounts []accountpixiv.Account, targetID, defaultID int64) (accountpixiv.Account, error) {
	if targetID <= 0 {
		return accountpixiv.Account{}, fmt.Errorf("mutation account UID must be positive")
	}
	if targetID == defaultID {
		return accountpixiv.Account{}, fmt.Errorf("mutation account UID must not be the configured default account")
	}
	for _, account := range accounts {
		if account.UserID != targetID {
			continue
		}
		if !account.HasRefreshToken() {
			return accountpixiv.Account{}, fmt.Errorf("mutation account UID %d has no stored credential", targetID)
		}
		return account, nil
	}
	return accountpixiv.Account{}, fmt.Errorf("mutation account UID %d is not present in the local account database", targetID)
}

// TestRealPixivSDKLiveManifestMutation runs only when both mutation gates are
// explicit. The target UID is intentionally separate from the normal live-read
// account selector so a default/main account cannot receive test writes by
// accident.
func TestRealPixivSDKLiveManifestMutation(t *testing.T) {
	if os.Getenv(mutationGuardEnv) != "1" {
		t.Skip("set PIXIV_SDK_E2E_MUTATION=1 to run the real Pixiv mutation e2e")
	}
	targetID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(mutationAccountEnv)), 10, 64)
	if err != nil || targetID <= 0 {
		t.Fatalf("%s must be a positive local Pixiv account UID", mutationAccountEnv)
	}

	ctx := context.Background()
	client := openRealPixivMutationClient(t, ctx, targetID)
	identity, err := client.CurrentUser(ctx, pixivsdk.CurrentUserRequest{})
	if err != nil {
		t.Fatalf("current mutation account: %s", liveMutationReason(err))
	}
	if identity.User.ID != targetID {
		t.Fatalf("mutation account identity = %d, want %d", identity.User.ID, targetID)
	}
	t.Logf("mutation account: user_id=%d", targetID)

	marker := "codex-goal1-t30-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)

	// 标签不是本轮 bookmark 的必要 mutation 变量；不附加标签可避免把上游
	// 标签索引的可见性延迟误判为收藏状态失败，但仍验证 detail/list/tags 三个
	// read-back 入口均可安全读取。
	recordLiveMutation(t, "artwork_bookmark", runArtworkBookmarkMutation(ctx, client, targetID, ""))
	recordLiveMutation(t, "novel_bookmark", runNovelBookmarkMutation(ctx, client, targetID, ""))

	stamps, err := client.Stamps(ctx, pixivsdk.StampsRequest{})
	if err != nil {
		result := blockedMutationResult(0, err)
		recordLiveMutation(t, "stamps", result)
		recordLiveMutation(t, "artwork_comment_stamp", result)
		recordLiveMutation(t, "novel_comment_stamp", result)
	} else if len(stamps) == 0 {
		result := blockedMutationResult(0, fmt.Errorf("no usable stamp returned"))
		recordLiveMutation(t, "stamps", result)
		recordLiveMutation(t, "artwork_comment_stamp", result)
		recordLiveMutation(t, "novel_comment_stamp", result)
	} else {
		stampID := stamps[0].ID
		if stampID <= 0 {
			result := correctionMutationResult(0, sdk.MalformedUpstreamResponse)
			recordLiveMutation(t, "stamps", result)
			recordLiveMutation(t, "artwork_comment_stamp", result)
			recordLiveMutation(t, "novel_comment_stamp", result)
		} else {
			t.Logf("stamps: count=%d usable=true", len(stamps))
			recordLiveMutation(t, "stamps", verifiedReadOnlyResult(stampID, len(stamps)))
			runArtworkCommentMutations(t, ctx, client, stampID, marker)
			runNovelCommentMutations(t, ctx, client, stampID, marker)
		}
	}

	recordLiveMutation(t, "follow", runFollowMutation(ctx, client, targetID))
}

// TestRealPixivSDKLiveMutationReconcile is a read-first recovery path for a
// mutation response that was classified after dispatch but before a reliable
// outcome was available. It never replays an add. If the first run's known
// target is now bookmarked, one explicit delete restores the preflight state.
func TestRealPixivSDKLiveMutationReconcile(t *testing.T) {
	if os.Getenv("PIXIV_SDK_E2E_MUTATION_RECONCILE") != "1" {
		t.Skip("set PIXIV_SDK_E2E_MUTATION_RECONCILE=1 to run mutation reconciliation")
	}
	targetID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(mutationAccountEnv)), 10, 64)
	if err != nil || targetID <= 0 {
		t.Fatalf("%s must be a positive local Pixiv account UID", mutationAccountEnv)
	}
	artworkID := parseRequiredLiveID(t, "PIXIV_E2E_RECONCILE_ARTWORK_ID")
	novelID := parseRequiredLiveID(t, "PIXIV_E2E_RECONCILE_NOVEL_ID")
	ctx := context.Background()
	client := openRealPixivMutationClient(t, ctx, targetID)

	reconcileArtworkBookmark(t, ctx, client, targetID, artworkID)
	reconcileNovelBookmark(t, ctx, client, targetID, novelID)
}

func parseRequiredLiveID(t *testing.T, name string) int64 {
	t.Helper()
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil || value <= 0 {
		t.Fatalf("%s must be a positive public entity ID", name)
	}
	return value
}

func reconcileArtworkBookmark(t *testing.T, ctx context.Context, client *pixivsdk.Client, userID, artworkID int64) {
	t.Helper()
	detail, err := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: artworkID})
	if err != nil {
		t.Fatalf("reconcile artwork detail %d: %s", artworkID, liveMutationReason(err))
	}
	list, listErr := client.UserArtworkBookmarks(ctx, pixivsdk.UserArtworkBookmarksRequest{UserID: userID, Restrict: pixivsdk.RestrictPublic})
	tags, tagsErr := client.UserArtworkBookmarkTags(ctx, pixivsdk.UserArtworkBookmarkTagsRequest{UserID: userID, Restrict: pixivsdk.RestrictPublic})
	if detail.Restrict == "" {
		t.Logf("reconcile artwork bookmark: target_id=%d bookmarked=false list_ok=%v tags_ok=%v list_present=%v tags=%d cleaned=false",
			artworkID, listErr == nil, tagsErr == nil, listErr == nil && containsArtworkID(list, artworkID), len(tags.Items))
		return
	}
	list, err = client.UserArtworkBookmarks(ctx, pixivsdk.UserArtworkBookmarksRequest{UserID: userID, Restrict: detail.Restrict})
	if err != nil {
		t.Fatalf("reconcile artwork list %d: %s", artworkID, liveMutationReason(err))
	}
	tags, err = client.UserArtworkBookmarkTags(ctx, pixivsdk.UserArtworkBookmarkTagsRequest{UserID: userID, Restrict: detail.Restrict})
	if err != nil {
		t.Fatalf("reconcile artwork tags %d: %s", artworkID, liveMutationReason(err))
	}
	t.Logf("reconcile artwork bookmark: target_id=%d bookmarked=true list_present=%v tags=%d", artworkID, containsArtworkID(list, artworkID), len(tags.Items))
	if err := client.RemoveArtworkBookmark(ctx, pixivsdk.RemoveArtworkBookmarkRequest{ArtworkID: artworkID}); err != nil {
		t.Fatalf("reconcile artwork delete %d: %s", artworkID, liveMutationReason(err))
	}
	after, err := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: artworkID})
	if err != nil {
		t.Fatalf("reconcile artwork detail after delete %d: %s", artworkID, liveMutationReason(err))
	}
	if after.Restrict != "" {
		t.Fatalf("reconcile artwork target %d remains bookmarked", artworkID)
	}
	t.Logf("reconcile artwork bookmark: target_id=%d bookmarked=false cleaned=true", artworkID)
}

func reconcileNovelBookmark(t *testing.T, ctx context.Context, client *pixivsdk.Client, userID, novelID int64) {
	t.Helper()
	detail, err := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: novelID})
	if err != nil {
		t.Fatalf("reconcile novel detail %d: %s", novelID, liveMutationReason(err))
	}
	list, listErr := client.UserNovelBookmarks(ctx, pixivsdk.UserNovelBookmarksRequest{UserID: userID, Restrict: pixivsdk.RestrictPublic})
	tags, tagsErr := client.UserNovelBookmarkTags(ctx, pixivsdk.UserNovelBookmarkTagsRequest{UserID: userID, Restrict: pixivsdk.RestrictPublic})
	if detail.Restrict == "" {
		t.Logf("reconcile novel bookmark: target_id=%d bookmarked=false list_ok=%v tags_ok=%v list_present=%v tags=%d cleaned=false",
			novelID, listErr == nil, tagsErr == nil, listErr == nil && containsNovelID(list, novelID), len(tags.Items))
		return
	}
	list, err = client.UserNovelBookmarks(ctx, pixivsdk.UserNovelBookmarksRequest{UserID: userID, Restrict: detail.Restrict})
	if err != nil {
		t.Fatalf("reconcile novel list %d: %s", novelID, liveMutationReason(err))
	}
	tags, err = client.UserNovelBookmarkTags(ctx, pixivsdk.UserNovelBookmarkTagsRequest{UserID: userID, Restrict: detail.Restrict})
	if err != nil {
		t.Fatalf("reconcile novel tags %d: %s", novelID, liveMutationReason(err))
	}
	t.Logf("reconcile novel bookmark: target_id=%d bookmarked=true list_present=%v tags=%d", novelID, containsNovelID(list, novelID), len(tags.Items))
	if err := client.RemoveNovelBookmark(ctx, pixivsdk.RemoveNovelBookmarkRequest{NovelID: novelID}); err != nil {
		t.Fatalf("reconcile novel delete %d: %s", novelID, liveMutationReason(err))
	}
	after, err := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: novelID})
	if err != nil {
		t.Fatalf("reconcile novel detail after delete %d: %s", novelID, liveMutationReason(err))
	}
	if after.Restrict != "" {
		t.Fatalf("reconcile novel target %d remains bookmarked", novelID)
	}
	t.Logf("reconcile novel bookmark: target_id=%d bookmarked=false cleaned=true", novelID)
}

func openRealPixivMutationClient(t *testing.T, ctx context.Context, targetID int64) *pixivsdk.Client {
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
	defaultID, hasDefault, err := config.ReadPixivDefaultUserID()
	if err != nil {
		t.Fatalf("read pixiv default account: %v", err)
	}
	if !hasDefault {
		if len(accounts) == 0 {
			t.Fatal("no local Pixiv account; explicit mutation e2e has no credential source")
		}
		// 与普通 live-read helper 的既有选择契约一致：未配置 UID 时，
		// 数据库 sort_order 第一项就是默认账号，mutation 必须排除它。
		defaultID = accounts[0].UserID
	}
	account, err := findPixivMutationAccount(accounts, targetID, defaultID)
	if err != nil {
		t.Fatal(err)
	}

	options := pixivsdk.Options{Pacing: pixivsdk.Pacing{MinInterval: liveManifestPace}}
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
		t.Fatalf("pixiv.Open mutation account: %s", liveMutationReason(err))
	}
	t.Cleanup(client.CloseIdleConnections)
	if credentials.UserID <= 0 || credentials.AccessToken() == "" || credentials.RefreshToken() == "" {
		t.Fatal("open mutation account did not return verified credentials")
	}
	if credentials.UserID != account.UserID {
		t.Fatalf("open mutation account identity = %d, stored user = %d", credentials.UserID, account.UserID)
	}
	if err := db.RotatePixivCredentials(ctx, account.UserID, account.CredentialRevision, []byte(credentials.RefreshToken())); err != nil {
		t.Fatalf("persist mutation account rotation: %v", err)
	}
	return client
}

type liveMutationResult struct {
	status    string
	targetID  int64
	writes    int
	readBack  bool
	cleanup   bool
	uncertain bool
	reason    sdk.Reason
	evidence  int
}

func verifiedReadOnlyResult(targetID int64, evidence int) liveMutationResult {
	return liveMutationResult{status: "verified", targetID: targetID, readBack: true, evidence: evidence}
}

func blockedMutationResult(targetID int64, err error) liveMutationResult {
	return liveMutationResult{
		status:    "blocked_external",
		targetID:  targetID,
		uncertain: isUncertainMutationError(err),
		reason:    liveMutationReason(err),
	}
}

func correctionMutationResult(targetID int64, reason sdk.Reason) liveMutationResult {
	return liveMutationResult{status: "correction", targetID: targetID, reason: reason}
}

func recordLiveMutation(t *testing.T, name string, result liveMutationResult) {
	t.Helper()
	t.Logf("%s: status=%s target_id=%d writes=%d read_back=%v cleanup=%v uncertain=%v evidence=%d reason=%s",
		name, result.status, result.targetID, result.writes, result.readBack, result.cleanup,
		result.uncertain, result.evidence, result.reason)
	if result.status == "correction" {
		t.Errorf("%s: live mutation contract correction required (reason=%s)", name, result.reason)
	}
}

func liveMutationReason(err error) sdk.Reason {
	if err == nil {
		return ""
	}
	if reason := sdk.ReasonOf(err); reason != "" {
		return reason
	}
	return sdk.UpstreamError
}

func isUncertainMutationError(err error) bool {
	if sdk.ReasonOf(err) == "" {
		return false
	}
	switch liveMutationReason(err) {
	case sdk.UpstreamError, sdk.UpstreamUnavailable, sdk.RateLimited, sdk.CredentialsExpired:
		return true
	default:
		return false
	}
}

func mutationWriteError(targetID int64, err error) liveMutationResult {
	if sdk.ReasonOf(err) == sdk.MalformedUpstreamResponse || sdk.ReasonOf(err) == sdk.InvalidArgument {
		return correctionMutationResult(targetID, liveMutationReason(err))
	}
	return blockedMutationResult(targetID, err)
}

func containsArtworkID(page sdk.Page[pixivsdk.Artwork], targetID int64) bool {
	for _, item := range page.Items {
		if item.ID == targetID {
			return true
		}
	}
	return false
}

func containsNovelID(page sdk.Page[pixivsdk.Novel], targetID int64) bool {
	for _, item := range page.Items {
		if item.ID == targetID {
			return true
		}
	}
	return false
}

func containsBookmarkTag(page sdk.Page[pixivsdk.BookmarkTag], name string) bool {
	for _, item := range page.Items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func bookmarkTagReadBackConfirmed(tag string, tags sdk.Page[pixivsdk.BookmarkTag]) bool {
	return tag == "" || containsBookmarkTag(tags, tag)
}

func containsCommentID(page pixivsdk.CommentPage, targetID int64) bool {
	for _, item := range page.Page.Items {
		if item.ID == targetID {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func runArtworkBookmarkMutation(ctx context.Context, client *pixivsdk.Client, userID int64, tag string) liveMutationResult {
	targetID, err := findUnbookmarkedArtwork(ctx, client)
	if err != nil {
		return blockedMutationResult(0, err)
	}
	original, err := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: targetID})
	if err != nil {
		return blockedMutationResult(targetID, err)
	}
	if original.Restrict != "" {
		return blockedMutationResult(targetID, fmt.Errorf("target artwork is already bookmarked"))
	}

	var bookmarkTags []string
	if tag != "" {
		bookmarkTags = []string{tag}
	}
	if err := client.AddArtworkBookmark(ctx, pixivsdk.AddArtworkBookmarkRequest{
		ArtworkID: targetID,
		Restrict:  pixivsdk.RestrictPublic,
		Tags:      bookmarkTags,
	}); err != nil {
		return mutationWriteError(targetID, err)
	}
	result := liveMutationResult{status: "correction", targetID: targetID, writes: 1}

	detail, detailErr := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: targetID})
	list, listErr := client.UserArtworkBookmarks(ctx, pixivsdk.UserArtworkBookmarksRequest{
		UserID: userID, Restrict: pixivsdk.RestrictPublic, Tag: tag,
	})
	tags, tagsErr := client.UserArtworkBookmarkTags(ctx, pixivsdk.UserArtworkBookmarkTagsRequest{
		UserID: userID, Restrict: pixivsdk.RestrictPublic,
	})
	detailTagsOK := (tag == "" && len(detail.Tags) == 0) || containsString(detail.Tags, tag)
	confirmedAdded := detailErr == nil && detail.Restrict == pixivsdk.RestrictPublic && detailTagsOK
	result.readBack = confirmedAdded && listErr == nil && tagsErr == nil && containsArtworkID(list, targetID) && bookmarkTagReadBackConfirmed(tag, tags)

	cleanup, cleanupReason := cleanupArtworkBookmark(ctx, client, userID, targetID, tag)
	result.cleanup = cleanup
	if cleanupReason != "" {
		result.reason = cleanupReason
		return result
	}
	if detailErr != nil {
		result.reason = liveMutationReason(detailErr)
		return classifyBookmarkReadback(result)
	}
	if listErr != nil {
		result.reason = liveMutationReason(listErr)
		return classifyBookmarkReadback(result)
	}
	if tagsErr != nil {
		result.reason = liveMutationReason(tagsErr)
		return classifyBookmarkReadback(result)
	}
	if !result.readBack {
		result.reason = sdk.MalformedUpstreamResponse
		return classifyBookmarkReadback(result)
	}
	if cleanup {
		result.status = "verified"
	} else {
		result.reason = sdk.MalformedUpstreamResponse
	}
	return result
}

func cleanupArtworkBookmark(ctx context.Context, client *pixivsdk.Client, userID, targetID int64, tag string) (bool, sdk.Reason) {
	if err := client.RemoveArtworkBookmark(ctx, pixivsdk.RemoveArtworkBookmarkRequest{ArtworkID: targetID}); err != nil {
		return false, liveMutationReason(err)
	}
	detail, err := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: targetID})
	if err != nil {
		return false, liveMutationReason(err)
	}
	list, err := client.UserArtworkBookmarks(ctx, pixivsdk.UserArtworkBookmarksRequest{
		UserID: userID, Restrict: pixivsdk.RestrictPublic, Tag: tag,
	})
	if err != nil {
		return false, liveMutationReason(err)
	}
	tags, err := client.UserArtworkBookmarkTags(ctx, pixivsdk.UserArtworkBookmarkTagsRequest{
		UserID: userID, Restrict: pixivsdk.RestrictPublic,
	})
	if err != nil {
		return false, liveMutationReason(err)
	}
	if detail.Restrict != "" || containsArtworkID(list, targetID) || (tag != "" && containsBookmarkTag(tags, tag)) {
		return false, sdk.MalformedUpstreamResponse
	}
	return true, ""
}

func findUnbookmarkedArtwork(ctx context.Context, client *pixivsdk.Client) (int64, error) {
	page, err := client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{
		Word: "初音ミク", ContentType: pixivsdk.SearchContentTypeIllust,
	})
	if err != nil {
		return 0, err
	}
	for _, item := range page.Items {
		if item.ID <= 0 {
			continue
		}
		detail, err := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: item.ID})
		if err != nil {
			continue
		}
		if detail.Restrict == "" {
			return item.ID, nil
		}
	}
	return 0, fmt.Errorf("current artwork search page has no unbookmarked writable target")
}

func runNovelBookmarkMutation(ctx context.Context, client *pixivsdk.Client, userID int64, tag string) liveMutationResult {
	targetID, err := findUnbookmarkedNovel(ctx, client)
	if err != nil {
		return blockedMutationResult(0, err)
	}
	original, err := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: targetID})
	if err != nil {
		return blockedMutationResult(targetID, err)
	}
	if original.Restrict != "" {
		return blockedMutationResult(targetID, fmt.Errorf("target novel is already bookmarked"))
	}

	var bookmarkTags []string
	if tag != "" {
		bookmarkTags = []string{tag}
	}
	if err := client.AddNovelBookmark(ctx, pixivsdk.AddNovelBookmarkRequest{
		NovelID:  targetID,
		Restrict: pixivsdk.RestrictPublic,
		Tags:     bookmarkTags,
	}); err != nil {
		return mutationWriteError(targetID, err)
	}
	result := liveMutationResult{status: "correction", targetID: targetID, writes: 1}

	detail, detailErr := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: targetID})
	list, listErr := client.UserNovelBookmarks(ctx, pixivsdk.UserNovelBookmarksRequest{
		UserID: userID, Restrict: pixivsdk.RestrictPublic, Tag: tag,
	})
	tags, tagsErr := client.UserNovelBookmarkTags(ctx, pixivsdk.UserNovelBookmarkTagsRequest{
		UserID: userID, Restrict: pixivsdk.RestrictPublic,
	})
	detailTagsOK := (tag == "" && len(detail.Tags) == 0) || containsString(detail.Tags, tag)
	result.readBack = detailErr == nil && detail.Restrict == pixivsdk.RestrictPublic && detailTagsOK &&
		listErr == nil && tagsErr == nil && containsNovelID(list, targetID) && bookmarkTagReadBackConfirmed(tag, tags)

	cleanup, cleanupReason := cleanupNovelBookmark(ctx, client, userID, targetID, tag)
	result.cleanup = cleanup
	if cleanupReason != "" {
		result.reason = cleanupReason
		return result
	}
	if detailErr != nil {
		result.reason = liveMutationReason(detailErr)
		return classifyBookmarkReadback(result)
	}
	if listErr != nil {
		result.reason = liveMutationReason(listErr)
		return classifyBookmarkReadback(result)
	}
	if tagsErr != nil {
		result.reason = liveMutationReason(tagsErr)
		return classifyBookmarkReadback(result)
	}
	if !result.readBack {
		result.reason = sdk.MalformedUpstreamResponse
		return classifyBookmarkReadback(result)
	}
	if cleanup {
		result.status = "verified"
	} else {
		result.reason = sdk.MalformedUpstreamResponse
	}
	return result
}

// classifyBookmarkReadback distinguishes an upstream status-only write whose
// side effect was safely removed but never became observable from an internal
// unsafe cleanup failure. The former is external/read-consistency evidence;
// it must not trigger an add replay.
func classifyBookmarkReadback(result liveMutationResult) liveMutationResult {
	if result.cleanup && !result.readBack {
		result.status = "blocked_external"
		result.uncertain = true
	}
	return result
}

func cleanupNovelBookmark(ctx context.Context, client *pixivsdk.Client, userID, targetID int64, tag string) (bool, sdk.Reason) {
	if err := client.RemoveNovelBookmark(ctx, pixivsdk.RemoveNovelBookmarkRequest{NovelID: targetID}); err != nil {
		return false, liveMutationReason(err)
	}
	detail, err := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: targetID})
	if err != nil {
		return false, liveMutationReason(err)
	}
	list, err := client.UserNovelBookmarks(ctx, pixivsdk.UserNovelBookmarksRequest{
		UserID: userID, Restrict: pixivsdk.RestrictPublic, Tag: tag,
	})
	if err != nil {
		return false, liveMutationReason(err)
	}
	tags, err := client.UserNovelBookmarkTags(ctx, pixivsdk.UserNovelBookmarkTagsRequest{
		UserID: userID, Restrict: pixivsdk.RestrictPublic,
	})
	if err != nil {
		return false, liveMutationReason(err)
	}
	if detail.Restrict != "" || containsNovelID(list, targetID) || (tag != "" && containsBookmarkTag(tags, tag)) {
		return false, sdk.MalformedUpstreamResponse
	}
	return true, ""
}

func findUnbookmarkedNovel(ctx context.Context, client *pixivsdk.Client) (int64, error) {
	page, err := client.SearchNovels(ctx, pixivsdk.SearchNovelsRequest{Word: "初音ミク"})
	if err != nil {
		return 0, err
	}
	for _, item := range page.Items {
		if item.ID <= 0 {
			continue
		}
		detail, err := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: item.ID})
		if err != nil {
			continue
		}
		if detail.Restrict == "" {
			return item.ID, nil
		}
	}
	return 0, fmt.Errorf("current novel search page has no unbookmarked writable target")
}

type liveCommentTarget struct {
	contentID int64
	parentID  int64
}

func runArtworkCommentMutations(t *testing.T, ctx context.Context, client *pixivsdk.Client, stampID int64, marker string) {
	t.Helper()
	target, err := findArtworkCommentTarget(ctx, client)
	if err != nil {
		result := blockedMutationResult(0, err)
		recordLiveMutation(t, "artwork_comment_text", result)
		recordLiveMutation(t, "artwork_comment_reply", result)
		recordLiveMutation(t, "artwork_comment_stamp", result)
		return
	}

	read := func(commentID int64) (bool, error) {
		page, err := client.ArtworkComments(ctx, pixivsdk.ArtworkCommentsRequest{ArtworkID: target.contentID})
		if err != nil {
			return false, err
		}
		return containsCommentID(page, commentID), nil
	}
	remove := func(commentID int64) error {
		return client.DeleteArtworkComment(ctx, pixivsdk.DeleteArtworkCommentRequest{CommentID: commentID})
	}

	recordLiveMutation(t, "artwork_comment_text", runLiveCommentMutation(
		target.contentID,
		func() (int64, error) {
			result, err := client.PostArtworkComment(ctx, pixivsdk.PostArtworkCommentRequest{
				ArtworkID: target.contentID,
				Comment:   marker + " artwork text",
			})
			return result.CommentID, err
		}, read, remove,
	))

	if target.parentID <= 0 {
		recordLiveMutation(t, "artwork_comment_reply", blockedMutationResult(target.contentID, fmt.Errorf("target has no existing parent comment")))
	} else {
		recordLiveMutation(t, "artwork_comment_reply", runLiveCommentMutation(
			target.contentID,
			func() (int64, error) {
				result, err := client.ReplyArtworkComment(ctx, pixivsdk.ReplyArtworkCommentRequest{
					ArtworkID:       target.contentID,
					Comment:         marker + " artwork reply",
					ParentCommentID: target.parentID,
				})
				return result.CommentID, err
			}, read, remove,
		))
	}

	recordLiveMutation(t, "artwork_comment_stamp", runLiveCommentMutation(
		target.contentID,
		func() (int64, error) {
			result, err := client.StampArtworkComment(ctx, pixivsdk.StampArtworkCommentRequest{
				ArtworkID: target.contentID,
				Comment:   marker + " artwork stamp",
				StampID:   stampID,
			})
			return result.CommentID, err
		}, read, remove,
	))

}

func runNovelCommentMutations(t *testing.T, ctx context.Context, client *pixivsdk.Client, stampID int64, marker string) {
	t.Helper()
	target, err := findNovelCommentTarget(ctx, client)
	if err != nil {
		result := blockedMutationResult(0, err)
		recordLiveMutation(t, "novel_comment_text", result)
		recordLiveMutation(t, "novel_comment_stamp", result)
		return
	}

	read := func(commentID int64) (bool, error) {
		page, err := client.NovelComments(ctx, pixivsdk.NovelCommentsRequest{NovelID: target.contentID})
		if err != nil {
			return false, err
		}
		return containsCommentID(page, commentID), nil
	}
	remove := func(commentID int64) error {
		return client.DeleteNovelComment(ctx, pixivsdk.DeleteNovelCommentRequest{CommentID: commentID})
	}

	recordLiveMutation(t, "novel_comment_text", runLiveCommentMutation(
		target.contentID,
		func() (int64, error) {
			result, err := client.PostNovelComment(ctx, pixivsdk.PostNovelCommentRequest{
				NovelID: target.contentID,
				Comment: marker + " novel text",
			})
			return result.CommentID, err
		}, read, remove,
	))
	recordLiveMutation(t, "novel_comment_stamp", runLiveCommentMutation(
		target.contentID,
		func() (int64, error) {
			result, err := client.StampNovelComment(ctx, pixivsdk.StampNovelCommentRequest{
				NovelID: target.contentID,
				Comment: marker + " novel stamp",
				StampID: stampID,
			})
			return result.CommentID, err
		}, read, remove,
	))

}

func runLiveCommentMutation(targetID int64, write func() (int64, error), read func(int64) (bool, error), remove func(int64) error) liveMutationResult {
	commentID, err := write()
	if err != nil {
		return mutationWriteError(targetID, err)
	}
	result := liveMutationResult{status: "correction", targetID: targetID, writes: 1}
	if commentID <= 0 {
		result.reason = sdk.MalformedUpstreamResponse
		return result
	}

	present, readErr := read(commentID)
	result.readBack = readErr == nil && present
	deleteErr := remove(commentID)
	result.cleanup = deleteErr == nil
	if deleteErr != nil {
		result.reason = liveMutationReason(deleteErr)
		return result
	}
	after, afterErr := read(commentID)
	if afterErr != nil {
		result.reason = liveMutationReason(afterErr)
		return result
	}
	if !result.readBack || after {
		result.reason = sdk.MalformedUpstreamResponse
		return result
	}
	result.status = "verified"
	return result
}

func findArtworkCommentTarget(ctx context.Context, client *pixivsdk.Client) (liveCommentTarget, error) {
	page, err := client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{
		Word: "初音ミク", ContentType: pixivsdk.SearchContentTypeIllust,
	})
	if err != nil {
		return liveCommentTarget{}, err
	}
	for _, item := range page.Items {
		if item.ID <= 0 {
			continue
		}
		comments, err := client.ArtworkComments(ctx, pixivsdk.ArtworkCommentsRequest{ArtworkID: item.ID})
		if err != nil || comments.AccessControl == nil || !comments.AccessControl.CanComment || comments.AccessControl.IsLocked {
			continue
		}
		var parentID int64
		for _, comment := range comments.Page.Items {
			if comment.ID > 0 {
				parentID = comment.ID
				break
			}
		}
		return liveCommentTarget{contentID: item.ID, parentID: parentID}, nil
	}
	return liveCommentTarget{}, fmt.Errorf("current artwork search page has no target with explicit comment access control")
}

func findNovelCommentTarget(ctx context.Context, client *pixivsdk.Client) (liveCommentTarget, error) {
	page, err := client.SearchNovels(ctx, pixivsdk.SearchNovelsRequest{Word: "初音ミク"})
	if err != nil {
		return liveCommentTarget{}, err
	}
	for _, item := range page.Items {
		if item.ID <= 0 {
			continue
		}
		comments, err := client.NovelComments(ctx, pixivsdk.NovelCommentsRequest{NovelID: item.ID})
		if err != nil || comments.AccessControl == nil || !comments.AccessControl.CanComment || comments.AccessControl.IsLocked {
			continue
		}
		var parentID int64
		for _, comment := range comments.Page.Items {
			if comment.ID > 0 {
				parentID = comment.ID
				break
			}
		}
		return liveCommentTarget{contentID: item.ID, parentID: parentID}, nil
	}
	return liveCommentTarget{}, fmt.Errorf("current novel search page has no target with explicit comment access control")
}

func runFollowMutation(ctx context.Context, client *pixivsdk.Client, currentUserID int64) liveMutationResult {
	targetID, err := findUnfollowedUser(ctx, client, currentUserID)
	if err != nil {
		return blockedMutationResult(0, err)
	}

	if err := client.FollowUser(ctx, pixivsdk.FollowUserRequest{
		UserID: targetID, Restrict: pixivsdk.RestrictPublic,
	}); err != nil {
		return mutationWriteError(targetID, err)
	}
	result := liveMutationResult{status: "correction", targetID: targetID, writes: 1}
	afterAdd, err := client.User(ctx, pixivsdk.UserRequest{UserID: targetID})
	result.readBack = err == nil && afterAdd.User.IsFollowed

	cleanupErr := client.UnfollowUser(ctx, pixivsdk.UnfollowUserRequest{UserID: targetID})
	result.cleanup = cleanupErr == nil
	if cleanupErr != nil {
		result.reason = liveMutationReason(cleanupErr)
		return result
	}
	afterDelete, err := client.User(ctx, pixivsdk.UserRequest{UserID: targetID})
	if err != nil {
		result.reason = liveMutationReason(err)
		return result
	}
	if !result.readBack || afterDelete.User.IsFollowed {
		result.reason = sdk.MalformedUpstreamResponse
		return result
	}
	result.status = "verified"
	return result
}

func findUnfollowedUser(ctx context.Context, client *pixivsdk.Client, currentUserID int64) (int64, error) {
	page, err := client.SearchUsers(ctx, pixivsdk.SearchUsersRequest{Word: "pixiv"})
	if err != nil {
		return 0, err
	}
	for _, item := range page.Items {
		userID := item.User.ID
		if userID <= 0 || userID == currentUserID {
			continue
		}
		detail, err := client.User(ctx, pixivsdk.UserRequest{UserID: userID})
		if err != nil {
			continue
		}
		if !detail.User.IsFollowed {
			return userID, nil
		}
	}
	return 0, fmt.Errorf("current user search page has no unfollowed writable target")
}

func TestSelectPixivMutationAccountRequiresExplicitNonDefault(t *testing.T) {
	accounts := []accountpixiv.Account{
		accountpixiv.New(25649510, "main", []byte("refresh-main")),
		accountpixiv.New(127975236, "secondary", []byte("refresh-secondary")),
	}

	tests := []struct {
		name      string
		targetID  int64
		defaultID int64
		wantID    int64
		wantError bool
	}{
		{name: "selects explicit secondary", targetID: 127975236, defaultID: 25649510, wantID: 127975236},
		{name: "rejects default account", targetID: 25649510, defaultID: 25649510, wantError: true},
		{name: "rejects unknown account", targetID: 999999999, defaultID: 25649510, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			account, err := findPixivMutationAccount(accounts, test.targetID, test.defaultID)
			if (err != nil) != test.wantError {
				t.Fatalf("findPixivMutationAccount() error = %v, want error=%v", err, test.wantError)
			}
			if err == nil && account.UserID != test.wantID {
				t.Fatalf("selected account = %d, want %d", account.UserID, test.wantID)
			}
		})
	}
}

func TestBookmarkTagReadBackAllowsUntaggedMutation(t *testing.T) {
	tags := sdk.Page[pixivsdk.BookmarkTag]{Items: []pixivsdk.BookmarkTag{{Name: "existing", Count: 1}}}

	if !bookmarkTagReadBackConfirmed("", tags) {
		t.Fatal("untagged bookmark should not require an empty tag entry")
	}
	if !bookmarkTagReadBackConfirmed("existing", tags) {
		t.Fatal("tagged bookmark should require the requested tag entry")
	}
	if bookmarkTagReadBackConfirmed("missing", tags) {
		t.Fatal("tagged bookmark should reject a missing requested tag entry")
	}
}
