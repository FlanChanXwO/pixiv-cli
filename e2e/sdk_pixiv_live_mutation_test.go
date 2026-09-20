package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/appapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/database"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixivsdk "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

const (
	mutationGuardEnv                  = "PIXIV_SDK_E2E_MUTATION"
	mutationAccountEnv                = "PIXIV_E2E_MUTATION_USER_ID"
	commentAccessControlProbeGuardEnv = "PIXIV_SDK_E2E_COMMENT_ACCESS_CONTROL_PROBE"
	commentMutationGuardEnv           = "PIXIV_SDK_E2E_COMMENT_MUTATION"
	// 评论清理由接口返回的 comment ID 完成，不需要把长 marker 写入用户可见内容。
	liveCommentBody = "很棒！"
)

func TestCommentReplyReadbackUsesDedicatedEndpoint(t *testing.T) {
	artworkPath, artworkKey := commentReplyReadback(protocol.AppIllustCommentAdd)
	if artworkPath != "/v2/illust/comment/replies" || artworkKey != "comment_id" {
		t.Fatalf("artwork reply read-back = (%q, %q)", artworkPath, artworkKey)
	}
	novelPath, novelKey := commentReplyReadback(protocol.AppNovelCommentAdd)
	if novelPath != "/v2/novel/comment/replies" || novelKey != "comment_id" {
		t.Fatalf("novel reply read-back = (%q, %q)", novelPath, novelKey)
	}
}

type commentAccessControlObservation struct {
	present bool
	kind    string
	value   *int64
}

func TestDecodeCommentAccessControlPreservesInteger(t *testing.T) {
	got, err := decodeCommentAccessControl([]byte(`{"comment_access_control":0}`))
	if err != nil {
		t.Fatalf("decode comment access control: %v", err)
	}
	if !got.present || got.kind != "integer" || got.value == nil || *got.value != 0 {
		t.Fatalf("observation = %#v, want present integer 0", got)
	}
}

func TestDecodeCommentAccessControlDoesNotInventObjectSemantics(t *testing.T) {
	got, err := decodeCommentAccessControl([]byte(`{"access_control":{"can_comment":true,"is_locked":false}}`))
	if err != nil {
		t.Fatalf("decode comment access control: %v", err)
	}
	if got.present || got.kind != "missing" || got.value != nil {
		t.Fatalf("observation = %#v, want missing scalar observation", got)
	}
}

func TestCommentTargetUnavailableUsesContentUnavailable(t *testing.T) {
	err := commentTargetUnavailable("artwork")
	if sdk.ReasonOf(err) != sdk.ContentUnavailable {
		t.Fatalf("reason = %q, want %q", sdk.ReasonOf(err), sdk.ContentUnavailable)
	}
}

func TestLiveMutationResultMustStopAfterUncertainWrite(t *testing.T) {
	tests := []struct {
		name   string
		result liveMutationResult
		want   bool
	}{
		{name: "uncertain after write", result: liveMutationResult{writes: 1, uncertain: true, cleanup: true}, want: true},
		{name: "cleanup not confirmed", result: liveMutationResult{writes: 1, readBack: true, cleanup: false}, want: true},
		{name: "correction after write", result: liveMutationResult{status: "correction", writes: 1, cleanup: true}, want: true},
		{name: "verified and cleaned", result: liveMutationResult{status: "verified", writes: 1, readBack: true, cleanup: true}, want: false},
		{name: "read-only correction", result: liveMutationResult{status: "correction"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := liveMutationMustStop(test.result); got != test.want {
				t.Fatalf("liveMutationMustStop(%+v) = %v, want %v", test.result, got, test.want)
			}
		})
	}
}

func TestCommentTargetProbePreservesMalformedCorrection(t *testing.T) {
	err := sdk.NewError("pixiv", "Comments", sdk.MalformedUpstreamResponse)
	result := commentTargetProbeResult(err)
	if result.status != "correction" || result.reason != sdk.MalformedUpstreamResponse {
		t.Fatalf("result = %#v, want malformed correction", result)
	}
}

func TestCommentTargetCanBeProbedWithUnknownAccessControl(t *testing.T) {
	page := sdk.Page[pixivsdk.Comment]{Items: []pixivsdk.Comment{{ID: 1}}}
	if !commentTargetCanBeProbed(pixivsdk.CommentPage{Page: page}) {
		t.Fatal("readable comment target with unknown access control should be probeable under explicit mutation gate")
	}
}

func TestCommentTargetCannotBeProbedWithoutComments(t *testing.T) {
	if commentTargetCanBeProbed(pixivsdk.CommentPage{}) {
		t.Fatal("empty comment page should not be a mutation target")
	}
}

func TestCommentTargetCanBeProbedPreservesOpaqueNumericAccessControl(t *testing.T) {
	for _, numeric := range []int64{0, 1} {
		numeric := numeric
		page := pixivsdk.CommentPage{AccessControl: &pixivsdk.CommentAccessControl{NumericValue: &numeric}}
		if !commentTargetCanBeProbed(page) {
			t.Fatalf("numeric comment_access_control=%d is opaque and must not be interpreted as denied", numeric)
		}
	}
}

func TestCommentTargetCanBeProbedHonorsLegacyObjectAccessControl(t *testing.T) {
	for _, test := range []struct {
		name string
		page pixivsdk.CommentPage
		want bool
	}{
		{name: "denied", page: pixivsdk.CommentPage{AccessControl: &pixivsdk.CommentAccessControl{}}, want: false},
		{name: "allowed", page: pixivsdk.CommentPage{AccessControl: &pixivsdk.CommentAccessControl{CanComment: true}}, want: true},
		{name: "locked", page: pixivsdk.CommentPage{AccessControl: &pixivsdk.CommentAccessControl{CanComment: true, IsLocked: true}}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := commentTargetCanBeProbed(test.page); got != test.want {
				t.Fatalf("commentTargetCanBeProbed(%+v) = %v, want %v", test.page.AccessControl, got, test.want)
			}
		})
	}
}

func TestLiveCommentMutationUsesSeparatePostDeleteRead(t *testing.T) {
	writeReads := 0
	cleanupReads := 0
	result := runLiveCommentMutation(
		42,
		func() (int64, error) { return 99, nil },
		func(commentID int64) (bool, error) {
			writeReads++
			return commentID == 99, nil
		},
		func(commentID int64) error {
			if commentID != 99 {
				t.Fatalf("delete comment id = %d, want 99", commentID)
			}
			return nil
		},
		func(commentID int64) (bool, error) {
			cleanupReads++
			return false, nil
		},
	)
	if result.status != "verified" || !result.readBack || !result.cleanup || result.writes != 1 {
		t.Fatalf("result = %+v, want verified write/read/delete/absence", result)
	}
	if writeReads != 1 || cleanupReads != 1 {
		t.Fatalf("read calls = write:%d cleanup:%d, want 1/1", writeReads, cleanupReads)
	}
}

func TestReplyCleanupReadTreatsOnlyNotFoundAsAbsent(t *testing.T) {
	present, err := normalizeReplyCleanupRead(false, protocol.HTTPStatus(http.StatusNotFound))
	if err != nil || present {
		t.Fatalf("404 cleanup read = present:%v err:%v, want absent without error", present, err)
	}
	present, err = normalizeReplyCleanupRead(false, protocol.HTTPStatus(http.StatusInternalServerError))
	if err == nil || present {
		t.Fatalf("500 cleanup read = present:%v err:%v, want preserved error", present, err)
	}
	var failure protocol.Failure
	if !errors.As(err, &failure) || failure.StatusCode != http.StatusInternalServerError {
		t.Fatalf("cleanup error = %v, want preserved HTTP 500 failure", err)
	}
}

func TestLiveCommentMutationKeepsWriteReadNotFoundStrict(t *testing.T) {
	result := runLiveCommentMutation(
		42,
		func() (int64, error) { return 99, nil },
		func(int64) (bool, error) { return false, protocol.HTTPStatus(http.StatusNotFound) },
		func(int64) error { return nil },
		func(int64) (bool, error) { return false, nil },
	)
	if result.status != "correction" || result.readBack || !result.cleanup {
		t.Fatalf("result = %+v, want strict write read-back failure with confirmed cleanup", result)
	}
}

func TestLiveCommentMutationRejectsPostDeletePresence(t *testing.T) {
	result := runLiveCommentMutation(
		42,
		func() (int64, error) { return 99, nil },
		func(int64) (bool, error) { return true, nil },
		func(int64) error { return nil },
		func(int64) (bool, error) { return true, nil },
	)
	if result.status != "correction" || result.cleanup {
		t.Fatalf("result = %+v, want cleanup unconfirmed while deleted comment remains visible", result)
	}
}

func TestValidateRequiredCommentMutationEvidenceRequiresEveryPublicMutation(t *testing.T) {
	verified := liveMutationResult{status: "verified", writes: 1, readBack: true, cleanup: true}
	all := []namedLiveMutationResult{
		{name: "artwork_comment_text", result: verified},
		{name: "artwork_comment_reply", result: verified},
		{name: "artwork_comment_stamp", result: verified},
		{name: "novel_comment_text", result: verified},
		{name: "novel_comment_reply", result: verified},
		{name: "novel_comment_stamp", result: verified},
	}
	if err := validateRequiredCommentMutationEvidence(all); err != nil {
		t.Fatalf("all verified evidence rejected: %v", err)
	}

	missingNovelReply := append([]namedLiveMutationResult(nil), all[:4]...)
	missingNovelReply = append(missingNovelReply, all[5])
	if err := validateRequiredCommentMutationEvidence(missingNovelReply); err == nil || !strings.Contains(err.Error(), "novel_comment_reply") {
		t.Fatalf("missing novel reply error = %v", err)
	}

	blocked := append([]namedLiveMutationResult(nil), all...)
	blocked[0].result = blockedMutationResult(0, sdk.NewError("pixiv", "Comments", sdk.ContentUnavailable))
	if err := validateRequiredCommentMutationEvidence(blocked); err == nil || !strings.Contains(err.Error(), "artwork_comment_text") {
		t.Fatalf("blocked required mutation error = %v", err)
	}
}

type artworkReplyLifecycleRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn artworkReplyLifecycleRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func artworkReplyLifecycleJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestArtworkCommentReplyWithCreatedParentUsesWritableTargetAndCleansBothWrites(t *testing.T) {
	const (
		artworkID = int64(42)
		userID    = int64(22)
		parentID  = int64(701)
		replyID   = int64(702)
	)
	parentPresent := false
	replyPresent := false

	roundTripper := artworkReplyLifecycleRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if err := request.ParseForm(); err != nil {
			return nil, err
		}
		switch request.URL.Path {
		case protocol.AppIllustCommentAdd:
			if request.PostForm.Get("illust_id") != strconv.FormatInt(artworkID, 10) {
				return nil, fmt.Errorf("illust_id = %q, want %d", request.PostForm.Get("illust_id"), artworkID)
			}
			if request.PostForm.Get("parent_comment_id") == "" {
				parentPresent = true
				return artworkReplyLifecycleJSONResponse(`{"comment_id":701}`), nil
			}
			if request.PostForm.Get("parent_comment_id") != strconv.FormatInt(parentID, 10) || !parentPresent {
				return nil, fmt.Errorf("reply did not target the created parent")
			}
			replyPresent = true
			return artworkReplyLifecycleJSONResponse(`{"comment_id":702}`), nil
		case protocol.AppIllustComments:
			body := `{"comments":[],"comment_access_control":0}`
			if parentPresent {
				body = `{"comments":[{"id":701,"date":"2026-09-20T00:00:00+00:00","user":{"id":22}}],"comment_access_control":0}`
			}
			return artworkReplyLifecycleJSONResponse(body), nil
		case "/v2/illust/comment/replies":
			if !replyPresent {
				return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: http.NoBody}, nil
			}
			return artworkReplyLifecycleJSONResponse(`{"comments":[{"id":702,"user":{"id":22}}]}`), nil
		case protocol.AppIllustCommentDelete:
			switch request.PostForm.Get("comment_id") {
			case strconv.FormatInt(replyID, 10):
				replyPresent = false
			case strconv.FormatInt(parentID, 10):
				parentPresent = false
			default:
				return nil, fmt.Errorf("unexpected delete comment_id=%q", request.PostForm.Get("comment_id"))
			}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody}, nil
		default:
			return nil, fmt.Errorf("unexpected request path %q", request.URL.Path)
		}
	})
	httpClient := &http.Client{Transport: roundTripper}
	client, err := pixivsdk.NewWith("token", pixivsdk.Options{HTTPClient: httpClient})
	if err != nil {
		t.Fatalf("NewWith: %v", err)
	}
	session := &realPixivMutationSession{
		client: client,
		raw: appapi.New(
			appapi.WithHTTPClient(httpClient),
			appapi.WithAccessToken("token"),
			appapi.WithUserID(userID),
		),
		userID: userID,
	}

	parentResult, replyResult := runArtworkCommentReplyWithCreatedParent(t.Context(), session, liveCommentTarget{contentID: artworkID})
	if parentResult.status != "verified" || parentResult.writes != 1 || !parentResult.readBack || !parentResult.cleanup || parentResult.uncertain {
		t.Fatalf("parent result = %+v, want verified parent lifecycle", parentResult)
	}
	if replyResult.status != "verified" || replyResult.writes != 1 || !replyResult.readBack || !replyResult.cleanup || replyResult.uncertain {
		t.Fatalf("reply result = %+v, want verified reply lifecycle", replyResult)
	}
	if parentPresent || replyPresent {
		t.Fatalf("cleanup left state parent=%v reply=%v", parentPresent, replyPresent)
	}
}

// TestRealPixivSDKLiveCommentAccessControlProbe 记录当前 wire scalar，不给任意
// 整数赋予业务含义。它与 mutation runner 分离：发现 scalar 只是只读证据，不是
// 权限 preflight。
func TestRealPixivSDKLiveCommentAccessControlProbe(t *testing.T) {
	if os.Getenv(commentAccessControlProbeGuardEnv) != "1" {
		t.Skip("set PIXIV_SDK_E2E_COMMENT_ACCESS_CONTROL_PROBE=1 to run the read-only Pixiv comment access-control probe")
	}
	targetID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(mutationAccountEnv)), 10, 64)
	if err != nil || targetID <= 0 {
		t.Fatalf("%s must be a positive local Pixiv account UID", mutationAccountEnv)
	}

	ctx := context.Background()
	session := openRealPixivMutationSession(t, ctx, targetID)
	probeLiveCommentAccessControl(t, ctx, session, "artwork", protocol.AppIllustComments, "illust_id")
	probeLiveCommentAccessControl(t, ctx, session, "novel", protocol.AppNovelComments, "novel_id")
}

// TestRealPixivSDKLiveCommentMutationSlice 只重跑被 comments wire correction
// 影响的 comments slice。故意不调用完整 mutation runner，不重放已 verified 的
// follow/stamp-read evidence，也不重放 uncertain bookmark 写入。
func TestRealPixivSDKLiveCommentMutationSlice(t *testing.T) {
	if os.Getenv(commentMutationGuardEnv) != "1" {
		t.Skip("set PIXIV_SDK_E2E_COMMENT_MUTATION=1 to run the isolated Pixiv comment mutation slice")
	}
	targetID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(mutationAccountEnv)), 10, 64)
	if err != nil || targetID <= 0 {
		t.Fatalf("%s must be a positive local Pixiv account UID", mutationAccountEnv)
	}

	ctx := context.Background()
	session := openRealPixivMutationSession(t, ctx, targetID)
	client := session.client
	stamps, err := client.Stamps(ctx, pixivsdk.StampsRequest{})
	if err != nil {
		t.Fatalf("load stamps for explicit comment mutation evidence: %v", err)
	}
	if len(stamps) == 0 || stamps[0].ID <= 0 {
		t.Fatal("explicit comment mutation evidence requires at least one usable stamp")
	}

	stampID := stamps[0].ID
	results := runArtworkCommentMutations(t, ctx, session, stampID)
	results = append(results, runNovelCommentMutations(t, ctx, session, stampID)...)
	if err := validateRequiredCommentMutationEvidence(results); err != nil {
		t.Fatalf("explicit live comment mutation evidence is incomplete: %v", err)
	}
}

func readRawCommentIDs(ctx context.Context, raw *appapi.Client, path, idKey string, contentID, userID int64) (map[int64]struct{}, error) {
	var envelope struct {
		Comments []struct {
			ID   int64 `json:"id"`
			User struct {
				ID int64 `json:"id"`
			} `json:"user"`
		} `json:"comments"`
	}
	if err := raw.GetJSON(ctx, path, url.Values{idKey: {strconv.FormatInt(contentID, 10)}}, &envelope); err != nil {
		return nil, err
	}
	ids := make(map[int64]struct{})
	for _, comment := range envelope.Comments {
		if comment.ID > 0 && comment.User.ID == userID {
			ids[comment.ID] = struct{}{}
		}
	}
	return ids, nil
}

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
	session := openRealPixivMutationSession(t, ctx, targetID)
	client := session.client
	identity, err := client.CurrentUser(ctx, pixivsdk.CurrentUserRequest{})
	if err != nil {
		t.Fatalf("current mutation account: %s", liveMutationReason(err))
	}
	if identity.User.ID != targetID {
		t.Fatalf("mutation account identity = %d, want %d", identity.User.ID, targetID)
	}
	t.Logf("mutation account: user_id=%d", targetID)

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
			runArtworkCommentMutations(t, ctx, session, stampID)
			runNovelCommentMutations(t, ctx, session, stampID)
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

type realPixivMutationSession struct {
	client *pixivsdk.Client
	raw    *appapi.Client
	userID int64
}

// liveProbePacingRoundTripper 让 raw diagnostic path 遵守 public SDK operation
// 的既有 live pacing contract。raw probe 仅为观察一个尚未建模的 wire field，
// 才绕过 SDK operation layer。
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

func openRealPixivMutationClient(t *testing.T, ctx context.Context, targetID int64) *pixivsdk.Client {
	t.Helper()
	return openRealPixivMutationSession(t, ctx, targetID).client
}

func openRealPixivMutationSession(t *testing.T, ctx context.Context, targetID int64) *realPixivMutationSession {
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

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxy := os.Getenv("PIXIV_E2E_PROXY"); proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			t.Fatalf("parse proxy: %v", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	httpClient := &http.Client{Transport: transport}
	options := pixivsdk.Options{HTTPClient: httpClient, Pacing: pixivsdk.Pacing{MinInterval: liveManifestPace}}
	client, credentials, err := pixivsdk.OpenWith(ctx, string(account.RefreshTokenCopy()), options)
	if err != nil {
		t.Fatalf("pixiv.Open mutation account: %s", liveMutationReason(err))
	}
	t.Cleanup(httpClient.CloseIdleConnections)
	if credentials.UserID <= 0 || credentials.AccessToken() == "" || credentials.RefreshToken() == "" {
		t.Fatal("open mutation account did not return verified credentials")
	}
	if credentials.UserID != account.UserID {
		t.Fatalf("open mutation account identity = %d, stored user = %d", credentials.UserID, account.UserID)
	}
	if err := db.RotatePixivCredentials(ctx, account.UserID, account.CredentialRevision, []byte(credentials.RefreshToken())); err != nil {
		t.Fatalf("persist mutation account rotation: %v", err)
	}
	rawHTTPClient := &http.Client{Transport: &liveProbePacingRoundTripper{
		inner:    transport.Clone(),
		interval: liveManifestPace,
	}}
	app := appapi.New(
		appapi.WithHTTPClient(rawHTTPClient),
		appapi.WithAccessToken(credentials.AccessToken()),
		appapi.WithUserID(credentials.UserID),
	)
	t.Cleanup(rawHTTPClient.CloseIdleConnections)
	return &realPixivMutationSession{client: client, raw: app, userID: credentials.UserID}
}

func decodeCommentAccessControl(body []byte) (commentAccessControlObservation, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return commentAccessControlObservation{}, fmt.Errorf("comments response is not a JSON object")
	}
	raw, ok := envelope["comment_access_control"]
	if !ok {
		return commentAccessControlObservation{kind: "missing"}, nil
	}
	observation := commentAccessControlObservation{present: true}
	if strings.TrimSpace(string(raw)) == "null" {
		observation.kind = "null"
		return observation, nil
	}
	var integer int64
	if err := json.Unmarshal(raw, &integer); err == nil {
		observation.kind = "integer"
		observation.value = &integer
		return observation, nil
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return commentAccessControlObservation{}, fmt.Errorf("comment_access_control value is malformed")
	}
	switch value.(type) {
	case float64:
		observation.kind = "number"
	case bool:
		observation.kind = "boolean"
	case string:
		observation.kind = "string"
	case []any:
		observation.kind = "array"
	case map[string]any:
		observation.kind = "object"
	default:
		observation.kind = "unknown"
	}
	return observation, nil
}

func commentTargetUnavailable(family string) error {
	return sdk.NewError("pixiv", "CommentTarget", sdk.ContentUnavailable,
		sdk.WithDetail(family+" comments have no explicit access-control target"))
}

func commentTargetProbeResult(err error) liveMutationResult {
	return mutationWriteError(0, err)
}

func probeLiveCommentAccessControl(t *testing.T, ctx context.Context, session *realPixivMutationSession, family, path, parameter string) {
	t.Helper()
	var ids []int64
	switch family {
	case "artwork":
		page, err := session.client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{
			Word: "初音ミク", ContentType: pixivsdk.SearchContentTypeIllust,
		})
		if err != nil {
			t.Fatalf("%s comment access-control candidate search: %s", family, liveMutationReason(err))
		}
		ids = make([]int64, 0, len(page.Items))
		for _, item := range page.Items {
			ids = append(ids, item.ID)
		}
	case "novel":
		page, err := session.client.SearchNovels(ctx, pixivsdk.SearchNovelsRequest{Word: "初音ミク"})
		if err != nil {
			t.Fatalf("%s comment access-control candidate search: %s", family, liveMutationReason(err))
		}
		ids = make([]int64, 0, len(page.Items))
		for _, item := range page.Items {
			ids = append(ids, item.ID)
		}
	default:
		t.Fatalf("unsupported comment access-control family %q", family)
	}

	counts := map[string]int{}
	readErrors := map[sdk.Reason]int{}
	var scalarValues []int64
	responses := 0
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		body, err := session.raw.GetRaw(ctx, path, url.Values{parameter: {strconv.FormatInt(id, 10)}})
		if err != nil {
			readErrors[liveMutationReason(err)]++
			continue
		}
		responses++
		observation, err := decodeCommentAccessControl(body)
		if err != nil {
			t.Errorf("%s comment access-control target_id=%d: %v", family, id, err)
			continue
		}
		counts[observation.kind]++
		if observation.value != nil {
			scalarValues = append(scalarValues, *observation.value)
		}
	}
	t.Logf("comment access-control probe: family=%s candidates=%d responses=%d read_errors=%v wire_kinds=%v scalar_values=%v semantics=unconfirmed",
		family, len(ids), responses, readErrors, counts, scalarValues)
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

type namedLiveMutationResult struct {
	name   string
	result liveMutationResult
}

var requiredCommentMutationNames = []string{
	"artwork_comment_text",
	"artwork_comment_reply",
	"artwork_comment_stamp",
	"novel_comment_text",
	"novel_comment_reply",
	"novel_comment_stamp",
}

func validateRequiredCommentMutationEvidence(results []namedLiveMutationResult) error {
	byName := make(map[string]liveMutationResult, len(results))
	for _, item := range results {
		byName[item.name] = item.result
	}
	for _, name := range requiredCommentMutationNames {
		result, ok := byName[name]
		if !ok {
			return fmt.Errorf("required comment mutation %s produced no evidence", name)
		}
		if result.status != "verified" || result.writes != 1 || !result.readBack || !result.cleanup || result.uncertain {
			return fmt.Errorf("required comment mutation %s is not verified: status=%s writes=%d read_back=%v cleanup=%v uncertain=%v reason=%s",
				name, result.status, result.writes, result.readBack, result.cleanup, result.uncertain, result.reason)
		}
	}
	return nil
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
	if liveMutationMustStop(result) {
		t.Fatalf("%s: live mutation outcome is unsafe to continue after write (status=%s cleanup=%v uncertain=%v reason=%s)",
			name, result.status, result.cleanup, result.uncertain, result.reason)
	}
	if result.status == "correction" {
		t.Errorf("%s: live mutation contract correction required (reason=%s)", name, result.reason)
	}
}

func liveMutationMustStop(result liveMutationResult) bool {
	return result.writes > 0 && (result.uncertain || !result.cleanup || result.status == "correction")
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
}

func commentReplyReadback(writePath string) (string, string) {
	if writePath == protocol.AppNovelCommentAdd {
		return "/v2/novel/comment/replies", "comment_id"
	}
	return "/v2/illust/comment/replies", "comment_id"
}

func readCommentReplies(ctx context.Context, raw *appapi.Client, writePath string, parentID, userID, commentID int64) (bool, error) {
	path, idKey := commentReplyReadback(writePath)
	ids, err := readRawCommentIDs(ctx, raw, path, idKey, parentID, userID)
	if err != nil {
		return false, err
	}
	_, ok := ids[commentID]
	return ok, nil
}

func normalizeReplyCleanupRead(present bool, err error) (bool, error) {
	if err == nil {
		return present, nil
	}
	var failure protocol.Failure
	if errors.As(err, &failure) && failure.Kind == protocol.FailureHTTPStatus && failure.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, err
}

func readCommentRepliesAfterDelete(ctx context.Context, raw *appapi.Client, writePath string, parentID, userID, commentID int64) (bool, error) {
	present, err := readCommentReplies(ctx, raw, writePath, parentID, userID, commentID)
	return normalizeReplyCleanupRead(present, err)
}

func runArtworkCommentReplyWithCreatedParent(ctx context.Context, session *realPixivMutationSession, target liveCommentTarget) (liveMutationResult, liveMutationResult) {
	client := session.client
	parent, err := client.PostArtworkComment(ctx, pixivsdk.PostArtworkCommentRequest{
		ArtworkID: target.contentID,
		Comment:   liveCommentBody,
	})
	if err != nil {
		result := mutationWriteError(target.contentID, err)
		if result.uncertain || result.status == "correction" {
			result.writes = 1
		}
		return result, blockedMutationResult(target.contentID, fmt.Errorf("artwork reply parent setup failed"))
	}

	parentResult := liveMutationResult{status: "correction", targetID: target.contentID, writes: 1}
	if parent.CommentID <= 0 {
		parentResult.reason = sdk.MalformedUpstreamResponse
		return parentResult, correctionMutationResult(target.contentID, sdk.MalformedUpstreamResponse)
	}

	readParent := func(commentID int64) (bool, error) {
		page, err := client.ArtworkComments(ctx, pixivsdk.ArtworkCommentsRequest{ArtworkID: target.contentID})
		if err != nil {
			return false, err
		}
		return containsCommentID(page, commentID), nil
	}
	remove := func(commentID int64) error {
		return client.DeleteArtworkComment(ctx, pixivsdk.DeleteArtworkCommentRequest{CommentID: commentID})
	}

	present, readErr := readParent(parent.CommentID)
	parentResult.readBack = readErr == nil && present
	if readErr != nil {
		parentResult.reason = liveMutationReason(readErr)
	} else if !present {
		parentResult.reason = sdk.MalformedUpstreamResponse
	}

	replyResult := blockedMutationResult(target.contentID, fmt.Errorf("artwork reply parent setup was not readable"))
	if parentResult.readBack {
		replyRead := func(commentID int64) (bool, error) {
			return readCommentReplies(ctx, session.raw, protocol.AppIllustCommentAdd, parent.CommentID, session.userID, commentID)
		}
		replyCleanupRead := func(commentID int64) (bool, error) {
			return readCommentRepliesAfterDelete(ctx, session.raw, protocol.AppIllustCommentAdd, parent.CommentID, session.userID, commentID)
		}
		replyResult = runLiveCommentMutation(
			target.contentID,
			func() (int64, error) {
				result, err := client.ReplyArtworkComment(ctx, pixivsdk.ReplyArtworkCommentRequest{
					ArtworkID:       target.contentID,
					ParentCommentID: parent.CommentID,
					Comment:         liveCommentBody,
				})
				return result.CommentID, err
			},
			replyRead,
			remove,
			replyCleanupRead,
		)
	}

	if err := remove(parent.CommentID); err != nil {
		parentResult.reason = liveMutationReason(err)
		return parentResult, replyResult
	}
	after, afterErr := readParent(parent.CommentID)
	if afterErr != nil {
		parentResult.reason = liveMutationReason(afterErr)
		return parentResult, replyResult
	}
	if after {
		parentResult.reason = sdk.MalformedUpstreamResponse
		return parentResult, replyResult
	}
	parentResult.cleanup = true
	if parentResult.readBack && parentResult.reason == "" {
		parentResult.status = "verified"
	}
	return parentResult, replyResult
}

func runNovelCommentReplyWithCreatedParent(ctx context.Context, session *realPixivMutationSession, target liveCommentTarget) (liveMutationResult, liveMutationResult) {
	client := session.client
	parent, err := client.PostNovelComment(ctx, pixivsdk.PostNovelCommentRequest{
		NovelID: target.contentID,
		Comment: liveCommentBody,
	})
	if err != nil {
		result := mutationWriteError(target.contentID, err)
		if result.uncertain || result.status == "correction" {
			result.writes = 1
		}
		return result, blockedMutationResult(target.contentID, fmt.Errorf("novel reply parent setup failed"))
	}

	parentResult := liveMutationResult{status: "correction", targetID: target.contentID, writes: 1}
	if parent.CommentID <= 0 {
		parentResult.reason = sdk.MalformedUpstreamResponse
		return parentResult, correctionMutationResult(target.contentID, sdk.MalformedUpstreamResponse)
	}

	readParent := func(commentID int64) (bool, error) {
		page, err := client.NovelComments(ctx, pixivsdk.NovelCommentsRequest{NovelID: target.contentID})
		if err != nil {
			return false, err
		}
		return containsCommentID(page, commentID), nil
	}
	remove := func(commentID int64) error {
		return client.DeleteNovelComment(ctx, pixivsdk.DeleteNovelCommentRequest{CommentID: commentID})
	}

	present, readErr := readParent(parent.CommentID)
	parentResult.readBack = readErr == nil && present
	if readErr != nil {
		parentResult.reason = liveMutationReason(readErr)
	} else if !present {
		parentResult.reason = sdk.MalformedUpstreamResponse
	}

	replyResult := blockedMutationResult(target.contentID, fmt.Errorf("novel reply parent setup was not readable"))
	if parentResult.readBack {
		replyRead := func(commentID int64) (bool, error) {
			return readCommentReplies(ctx, session.raw, protocol.AppNovelCommentAdd, parent.CommentID, session.userID, commentID)
		}
		replyCleanupRead := func(commentID int64) (bool, error) {
			return readCommentRepliesAfterDelete(ctx, session.raw, protocol.AppNovelCommentAdd, parent.CommentID, session.userID, commentID)
		}
		replyResult = runLiveCommentMutation(
			target.contentID,
			func() (int64, error) {
				result, err := client.ReplyNovelComment(ctx, pixivsdk.ReplyNovelCommentRequest{
					NovelID:         target.contentID,
					ParentCommentID: parent.CommentID,
					Comment:         liveCommentBody,
				})
				return result.CommentID, err
			},
			replyRead,
			remove,
			replyCleanupRead,
		)
	}

	if err := remove(parent.CommentID); err != nil {
		parentResult.reason = liveMutationReason(err)
		return parentResult, replyResult
	}
	after, afterErr := readParent(parent.CommentID)
	if afterErr != nil {
		parentResult.reason = liveMutationReason(afterErr)
		return parentResult, replyResult
	}
	if after {
		parentResult.reason = sdk.MalformedUpstreamResponse
		return parentResult, replyResult
	}
	parentResult.cleanup = true
	if parentResult.readBack && parentResult.reason == "" {
		parentResult.status = "verified"
	}
	return parentResult, replyResult
}

func runArtworkCommentMutations(t *testing.T, ctx context.Context, session *realPixivMutationSession, stampID int64) []namedLiveMutationResult {
	t.Helper()
	client := session.client
	results := make([]namedLiveMutationResult, 0, 3)
	record := func(name string, result liveMutationResult) {
		results = append(results, namedLiveMutationResult{name: name, result: result})
		recordLiveMutation(t, name, result)
	}

	target, err := findWritableArtworkCommentTarget(ctx, client)
	if err != nil {
		result := commentTargetProbeResult(err)
		record("artwork_comment_text", result)
		record("artwork_comment_stamp", result)
	} else {
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
		stampRead := func(commentID int64) (bool, error) {
			ids, err := readRawCommentIDs(ctx, session.raw, "/v3/illust/comments", "illust_id", target.contentID, session.userID)
			if err != nil {
				return false, err
			}
			_, ok := ids[commentID]
			return ok, nil
		}

		record("artwork_comment_text", runLiveCommentMutation(
			target.contentID,
			func() (int64, error) {
				result, err := client.PostArtworkComment(ctx, pixivsdk.PostArtworkCommentRequest{
					ArtworkID: target.contentID,
					Comment:   liveCommentBody,
				})
				return result.CommentID, err
			}, read, remove, read,
		))

		record("artwork_comment_stamp", runLiveCommentMutation(
			target.contentID,
			func() (int64, error) {
				result, err := client.StampArtworkComment(ctx, pixivsdk.StampArtworkCommentRequest{
					ArtworkID: target.contentID,
					Comment:   "",
					StampID:   stampID,
				})
				return result.CommentID, err
			}, stampRead, remove, stampRead,
		))
	}

	if err != nil {
		record("artwork_comment_reply", commentTargetProbeResult(err))
	} else {
		parentResult, replyResult := runArtworkCommentReplyWithCreatedParent(ctx, session, target)
		recordLiveMutation(t, "artwork_comment_reply_parent_setup", parentResult)
		record("artwork_comment_reply", replyResult)
	}

	return results
}

func runNovelCommentMutations(t *testing.T, ctx context.Context, session *realPixivMutationSession, stampID int64) []namedLiveMutationResult {
	t.Helper()
	client := session.client
	results := make([]namedLiveMutationResult, 0, 3)
	record := func(name string, result liveMutationResult) {
		results = append(results, namedLiveMutationResult{name: name, result: result})
		recordLiveMutation(t, name, result)
	}

	target, err := findWritableNovelCommentTarget(ctx, client)
	if err != nil {
		result := commentTargetProbeResult(err)
		record("novel_comment_text", result)
		record("novel_comment_stamp", result)
	} else {
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
		stampRead := func(commentID int64) (bool, error) {
			ids, err := readRawCommentIDs(ctx, session.raw, "/v3/novel/comments", "novel_id", target.contentID, session.userID)
			if err != nil {
				return false, err
			}
			_, ok := ids[commentID]
			return ok, nil
		}

		record("novel_comment_text", runLiveCommentMutation(
			target.contentID,
			func() (int64, error) {
				result, err := client.PostNovelComment(ctx, pixivsdk.PostNovelCommentRequest{
					NovelID: target.contentID,
					Comment: liveCommentBody,
				})
				return result.CommentID, err
			}, read, remove, read,
		))
		record("novel_comment_stamp", runLiveCommentMutation(
			target.contentID,
			func() (int64, error) {
				result, err := client.StampNovelComment(ctx, pixivsdk.StampNovelCommentRequest{
					NovelID: target.contentID,
					Comment: "",
					StampID: stampID,
				})
				return result.CommentID, err
			}, stampRead, remove, stampRead,
		))
	}

	if err != nil {
		record("novel_comment_reply", commentTargetProbeResult(err))
	} else {
		parentResult, replyResult := runNovelCommentReplyWithCreatedParent(ctx, session, target)
		recordLiveMutation(t, "novel_comment_reply_parent_setup", parentResult)
		record("novel_comment_reply", replyResult)
	}

	return results
}

func runLiveCommentMutation(targetID int64, write func() (int64, error), readAfterWrite func(int64) (bool, error), remove func(int64) error, readAfterDelete func(int64) (bool, error)) liveMutationResult {
	commentID, err := write()
	if err != nil {
		result := mutationWriteError(targetID, err)
		if result.uncertain || result.status == "correction" {
			result.writes = 1
		}
		return result
	}
	result := liveMutationResult{status: "correction", targetID: targetID, writes: 1}
	if commentID <= 0 {
		result.reason = sdk.MalformedUpstreamResponse
		return result
	}

	present, readErr := readAfterWrite(commentID)
	result.readBack = readErr == nil && present
	deleteErr := remove(commentID)
	if deleteErr != nil {
		result.reason = liveMutationReason(deleteErr)
		return result
	}
	after, afterErr := readAfterDelete(commentID)
	if afterErr != nil {
		result.reason = liveMutationReason(afterErr)
		return result
	}
	if after {
		result.reason = sdk.MalformedUpstreamResponse
		return result
	}
	result.cleanup = true
	if readErr != nil {
		result.reason = liveMutationReason(readErr)
		return result
	}
	if !result.readBack {
		result.reason = sdk.MalformedUpstreamResponse
		return result
	}
	result.status = "verified"
	return result
}

// commentTargetCanBeProbed only decides whether one explicit live write may be
// attempted. Unknown scalar access-control is never mapped to CanComment; the
// mutation test's explicit user authorization plus write/read-back/cleanup provide
// the target-specific evidence.
func commentTargetCanBeProbed(page pixivsdk.CommentPage) bool {
	if page.AccessControl != nil {
		if page.AccessControl.NumericValue != nil {
			return true
		}
		return page.AccessControl.CanComment && !page.AccessControl.IsLocked
	}
	for _, comment := range page.Page.Items {
		if comment.ID > 0 {
			return true
		}
	}
	return false
}

func findWritableArtworkCommentTarget(ctx context.Context, client *pixivsdk.Client) (liveCommentTarget, error) {
	page, err := client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{
		Word: "初音ミク", ContentType: pixivsdk.SearchContentTypeIllust,
	})
	if err != nil {
		return liveCommentTarget{}, err
	}
	var lastErr error
	readableCandidates := 0
	for _, item := range page.Items {
		if item.ID <= 0 {
			continue
		}
		comments, err := client.ArtworkComments(ctx, pixivsdk.ArtworkCommentsRequest{ArtworkID: item.ID})
		if err != nil {
			lastErr = err
			continue
		}
		readableCandidates++
		if !commentTargetCanBeProbed(comments) {
			continue
		}
		return liveCommentTarget{contentID: item.ID}, nil
	}
	if readableCandidates == 0 && lastErr != nil {
		return liveCommentTarget{}, lastErr
	}
	return liveCommentTarget{}, commentTargetUnavailable("artwork")
}

func findWritableNovelCommentTarget(ctx context.Context, client *pixivsdk.Client) (liveCommentTarget, error) {
	page, err := client.SearchNovels(ctx, pixivsdk.SearchNovelsRequest{Word: "初音ミク"})
	if err != nil {
		return liveCommentTarget{}, err
	}
	var lastErr error
	readableCandidates := 0
	for _, item := range page.Items {
		if item.ID <= 0 {
			continue
		}
		comments, err := client.NovelComments(ctx, pixivsdk.NovelCommentsRequest{NovelID: item.ID})
		if err != nil {
			lastErr = err
			continue
		}
		readableCandidates++
		if !commentTargetCanBeProbed(comments) {
			continue
		}
		return liveCommentTarget{contentID: item.ID}, nil
	}
	if readableCandidates == 0 && lastErr != nil {
		return liveCommentTarget{}, lastErr
	}
	return liveCommentTarget{}, commentTargetUnavailable("novel")
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
