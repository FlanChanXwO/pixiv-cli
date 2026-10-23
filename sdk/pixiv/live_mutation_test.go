package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	protocol "github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
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

type commentAccessControlObservation struct {
	present bool
	kind    string
	value   *int64
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
	session := &pixivLiveSession{
		client: client,
		raw: protocol.New(
			protocol.WithHTTPClient(httpClient),
			protocol.WithAccessToken("token"),
			protocol.WithUserID(userID),
		),
		userID: userID,
	}

	parentResult, replyResult := runCommentReplyWithCreatedParent(t.Context(), session, artworkCommentOps(session), liveCommentTarget{contentID: artworkID})
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
	targetID := mutationAccountID(t)

	ctx := context.Background()
	session := openPixivLiveSession(t, ctx, targetID)
	for _, ops := range []commentTargetOps{artworkCommentOps(session), novelCommentOps(session)} {
		probeLiveCommentAccessControl(t, ctx, session, ops)
	}
}

// TestRealPixivSDKLiveCommentMutationSlice 只重跑被 comments wire correction
// 影响的 comments slice。故意不调用完整 mutation runner，不重放已 verified 的
// follow/stamp-read evidence，也不重放 uncertain bookmark 写入。
func TestRealPixivSDKLiveCommentMutationSlice(t *testing.T) {
	if os.Getenv(commentMutationGuardEnv) != "1" {
		t.Skip("set PIXIV_SDK_E2E_COMMENT_MUTATION=1 to run the isolated Pixiv comment mutation slice")
	}
	targetID := mutationAccountID(t)

	ctx := context.Background()
	session := openPixivLiveSession(t, ctx, targetID)
	stamps, err := session.client.Stamps(ctx, pixivsdk.StampsRequest{})
	if err != nil {
		t.Fatalf("load stamps for explicit comment mutation evidence: %v", err)
	}
	if len(stamps) == 0 || stamps[0].ID <= 0 {
		t.Fatal("explicit comment mutation evidence requires at least one usable stamp")
	}

	stampID := stamps[0].ID
	var results []namedLiveMutationResult
	for _, ops := range []commentTargetOps{artworkCommentOps(session), novelCommentOps(session)} {
		results = append(results, runCommentMutations(t, ctx, session, ops, stampID)...)
	}
	if err := validateRequiredCommentMutationEvidence(results); err != nil {
		t.Fatalf("explicit live comment mutation evidence is incomplete: %v", err)
	}
}

func mutationAccountID(t *testing.T) int64 {
	t.Helper()
	targetID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(mutationAccountEnv)), 10, 64)
	if err != nil || targetID <= 0 {
		t.Fatalf("%s must be a positive local Pixiv account UID", mutationAccountEnv)
	}
	return targetID
}

func readRawCommentIDs(ctx context.Context, raw *protocol.Client, path, idKey string, contentID, userID int64) (map[int64]struct{}, error) {
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
	targetID := mutationAccountID(t)

	ctx := context.Background()
	session := openPixivLiveSession(t, ctx, targetID)
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
	for _, ops := range []bookmarkTargetOps{artworkBookmarkOps(client), novelBookmarkOps(client)} {
		recordLiveMutation(t, ops.family+"_bookmark", runBookmarkMutation(ctx, client, session.userID, "", ops))
	}

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
			runCommentMutations(t, ctx, session, artworkCommentOps(session), stampID)
			runCommentMutations(t, ctx, session, novelCommentOps(session), stampID)
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
	targetID := mutationAccountID(t)
	targets := []struct {
		name string
		id   int64
		ops  func(*pixivsdk.Client) bookmarkTargetOps
	}{
		{name: "artwork", id: parseRequiredLiveID(t, "PIXIV_E2E_RECONCILE_ARTWORK_ID"), ops: artworkBookmarkOps},
		{name: "novel", id: parseRequiredLiveID(t, "PIXIV_E2E_RECONCILE_NOVEL_ID"), ops: novelBookmarkOps},
	}

	ctx := context.Background()
	session := openPixivLiveSession(t, ctx, targetID)
	for _, target := range targets {
		reconcileBookmark(t, ctx, session.client, session.userID, target.id, target.ops(session.client))
	}
}

func parseRequiredLiveID(t *testing.T, name string) int64 {
	t.Helper()
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil || value <= 0 {
		t.Fatalf("%s must be a positive public entity ID", name)
	}
	return value
}

func reconcileBookmark(t *testing.T, ctx context.Context, client *pixivsdk.Client, userID, targetID int64, ops bookmarkTargetOps) {
	t.Helper()
	restrict, tags, err := ops.detail(ctx, targetID)
	if err != nil {
		t.Fatalf("reconcile %s detail %d: %s", ops.family, targetID, liveMutationReason(err))
	}
	if restrict == "" {
		present, listErr := ops.contains(ctx, userID, pixivsdk.RestrictPublic, "", targetID)
		t.Logf("reconcile %s bookmark: target_id=%d bookmarked=false list_ok=%v tags=%d list_present=%v cleaned=false",
			ops.family, targetID, listErr == nil, len(tags), present)
		return
	}
	present, err := ops.contains(ctx, userID, restrict, "", targetID)
	if err != nil {
		t.Fatalf("reconcile %s list %d: %s", ops.family, targetID, liveMutationReason(err))
	}
	t.Logf("reconcile %s bookmark: target_id=%d bookmarked=true list_present=%v tags=%d", ops.family, targetID, present, len(tags))
	if err := ops.remove(ctx, targetID); err != nil {
		t.Fatalf("reconcile %s delete %d: %s", ops.family, targetID, liveMutationReason(err))
	}
	restrict, _, err = ops.detail(ctx, targetID)
	if err != nil {
		t.Fatalf("reconcile %s detail after delete %d: %s", ops.family, targetID, liveMutationReason(err))
	}
	if restrict != "" {
		t.Fatalf("reconcile %s target %d remains bookmarked", ops.family, targetID)
	}
	t.Logf("reconcile %s bookmark: target_id=%d bookmarked=false cleaned=true", ops.family, targetID)
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

func probeLiveCommentAccessControl(t *testing.T, ctx context.Context, session *pixivLiveSession, ops commentTargetOps) {
	t.Helper()
	ids, err := ops.search(ctx)
	if err != nil {
		t.Fatalf("%s comment access-control candidate search: %s", ops.family, liveMutationReason(err))
	}

	counts := map[string]int{}
	readErrors := map[sdk.Reason]int{}
	var scalarValues []int64
	responses := 0
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		body, err := session.raw.GetRaw(ctx, ops.rawCommentsPath, url.Values{ops.rawIDParameter: {strconv.FormatInt(id, 10)}})
		if err != nil {
			readErrors[liveMutationReason(err)]++
			continue
		}
		responses++
		observation, err := decodeCommentAccessControl(body)
		if err != nil {
			t.Errorf("%s comment access-control target_id=%d: %v", ops.family, id, err)
			continue
		}
		counts[observation.kind]++
		if observation.value != nil {
			scalarValues = append(scalarValues, *observation.value)
		}
	}
	t.Logf("comment access-control probe: family=%s candidates=%d responses=%d read_errors=%v wire_kinds=%v scalar_values=%v semantics=unconfirmed",
		ops.family, len(ids), responses, readErrors, counts, scalarValues)
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

// bookmarkTargetOps 只描述 artwork 与 novel 收藏不同的请求入口；写入、回读、
// 清理与停止条件由 runBookmarkMutation 共用一份。
type bookmarkTargetOps struct {
	family   string
	find     func(context.Context) (int64, error)
	detail   func(context.Context, int64) (pixivsdk.Restrict, []string, error)
	contains func(context.Context, int64, pixivsdk.Restrict, string, int64) (bool, error)
	tags     func(context.Context, int64, pixivsdk.Restrict) ([]string, error)
	add      func(context.Context, int64, string) error
	remove   func(context.Context, int64) error
}

func artworkBookmarkOps(client *pixivsdk.Client) bookmarkTargetOps {
	return bookmarkTargetOps{
		family: "artwork",
		find: func(ctx context.Context) (int64, error) {
			return findUnbookmarked(ctx, "artwork", func(ctx context.Context) (sdk.Page[pixivsdk.Artwork], error) {
				return client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{Word: "初音ミク", ContentType: pixivsdk.SearchContentTypeIllust})
			}, func(item pixivsdk.Artwork) int64 { return item.ID }, func(ctx context.Context, id int64) (pixivsdk.Restrict, error) {
				detail, err := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: id})
				return detail.Restrict, err
			})
		},
		detail: func(ctx context.Context, id int64) (pixivsdk.Restrict, []string, error) {
			detail, err := client.ArtworkBookmark(ctx, pixivsdk.ArtworkBookmarkRequest{ArtworkID: id})
			return detail.Restrict, detail.Tags, err
		},
		contains: func(ctx context.Context, userID int64, restrict pixivsdk.Restrict, tag string, targetID int64) (bool, error) {
			page, err := client.UserArtworkBookmarks(ctx, pixivsdk.UserArtworkBookmarksRequest{UserID: userID, Restrict: restrict, Tag: tag})
			return slices.ContainsFunc(page.Items, func(item pixivsdk.Artwork) bool { return item.ID == targetID }), err
		},
		tags: func(ctx context.Context, userID int64, restrict pixivsdk.Restrict) ([]string, error) {
			page, err := client.UserArtworkBookmarkTags(ctx, pixivsdk.UserArtworkBookmarkTagsRequest{UserID: userID, Restrict: restrict})
			return bookmarkTagNames(page), err
		},
		add: func(ctx context.Context, id int64, tag string) error {
			return client.AddArtworkBookmark(ctx, pixivsdk.AddArtworkBookmarkRequest{ArtworkID: id, Restrict: pixivsdk.RestrictPublic, Tags: tagNames(tag)})
		},
		remove: func(ctx context.Context, id int64) error {
			return client.RemoveArtworkBookmark(ctx, pixivsdk.RemoveArtworkBookmarkRequest{ArtworkID: id})
		},
	}
}

func novelBookmarkOps(client *pixivsdk.Client) bookmarkTargetOps {
	return bookmarkTargetOps{
		family: "novel",
		find: func(ctx context.Context) (int64, error) {
			return findUnbookmarked(ctx, "novel", func(ctx context.Context) (sdk.Page[pixivsdk.Novel], error) {
				return client.SearchNovels(ctx, pixivsdk.SearchNovelsRequest{Word: "初音ミク"})
			}, func(item pixivsdk.Novel) int64 { return item.ID }, func(ctx context.Context, id int64) (pixivsdk.Restrict, error) {
				detail, err := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: id})
				return detail.Restrict, err
			})
		},
		detail: func(ctx context.Context, id int64) (pixivsdk.Restrict, []string, error) {
			detail, err := client.NovelBookmark(ctx, pixivsdk.NovelBookmarkRequest{NovelID: id})
			return detail.Restrict, detail.Tags, err
		},
		contains: func(ctx context.Context, userID int64, restrict pixivsdk.Restrict, tag string, targetID int64) (bool, error) {
			page, err := client.UserNovelBookmarks(ctx, pixivsdk.UserNovelBookmarksRequest{UserID: userID, Restrict: restrict, Tag: tag})
			return slices.ContainsFunc(page.Items, func(item pixivsdk.Novel) bool { return item.ID == targetID }), err
		},
		tags: func(ctx context.Context, userID int64, restrict pixivsdk.Restrict) ([]string, error) {
			page, err := client.UserNovelBookmarkTags(ctx, pixivsdk.UserNovelBookmarkTagsRequest{UserID: userID, Restrict: restrict})
			return bookmarkTagNames(page), err
		},
		add: func(ctx context.Context, id int64, tag string) error {
			return client.AddNovelBookmark(ctx, pixivsdk.AddNovelBookmarkRequest{NovelID: id, Restrict: pixivsdk.RestrictPublic, Tags: tagNames(tag)})
		},
		remove: func(ctx context.Context, id int64) error {
			return client.RemoveNovelBookmark(ctx, pixivsdk.RemoveNovelBookmarkRequest{NovelID: id})
		},
	}
}

// findUnbookmarked 取搜索页第一个可写、尚未收藏的目标；跳过读取失败的候选，
// 只在当前页没有可写目标时失败。
func findUnbookmarked[T any](ctx context.Context, family string, search func(context.Context) (sdk.Page[T], error), idOf func(T) int64, detail func(context.Context, int64) (pixivsdk.Restrict, error)) (int64, error) {
	page, err := search(ctx)
	if err != nil {
		return 0, err
	}
	for _, item := range page.Items {
		id := idOf(item)
		if id <= 0 {
			continue
		}
		restrict, err := detail(ctx, id)
		if err != nil {
			continue
		}
		if restrict == "" {
			return id, nil
		}
	}
	return 0, fmt.Errorf("current %s search page has no unbookmarked writable target", family)
}

func tagNames(tag string) []string {
	if tag == "" {
		return nil
	}
	return []string{tag}
}

func bookmarkTagNames(page sdk.Page[pixivsdk.BookmarkTag]) []string {
	return slices.Collect(func(yield func(string) bool) {
		for _, item := range page.Items {
			if !yield(item.Name) {
				return
			}
		}
	})
}

func runBookmarkMutation(ctx context.Context, client *pixivsdk.Client, userID int64, tag string, ops bookmarkTargetOps) liveMutationResult {
	targetID, err := ops.find(ctx)
	if err != nil {
		return blockedMutationResult(0, err)
	}
	restrict, _, err := ops.detail(ctx, targetID)
	if err != nil {
		return blockedMutationResult(targetID, err)
	}
	if restrict != "" {
		return blockedMutationResult(targetID, fmt.Errorf("target %s is already bookmarked", ops.family))
	}

	if err := ops.add(ctx, targetID, tag); err != nil {
		return mutationWriteError(targetID, err)
	}
	result := liveMutationResult{status: "correction", targetID: targetID, writes: 1}

	detail, detailTags, detailErr := ops.detail(ctx, targetID)
	contains, listErr := ops.contains(ctx, userID, pixivsdk.RestrictPublic, tag, targetID)
	tags, tagsErr := ops.tags(ctx, userID, pixivsdk.RestrictPublic)
	detailTagsOK := (tag == "" && len(detailTags) == 0) || slices.Contains(detailTags, tag)
	result.readBack = detailErr == nil && detail == pixivsdk.RestrictPublic && detailTagsOK &&
		listErr == nil && tagsErr == nil && contains && (tag == "" || slices.Contains(tags, tag))

	cleanup, cleanupReason := cleanupBookmark(ctx, client, userID, targetID, tag, ops)
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

func cleanupBookmark(ctx context.Context, client *pixivsdk.Client, userID, targetID int64, tag string, ops bookmarkTargetOps) (bool, sdk.Reason) {
	if err := ops.remove(ctx, targetID); err != nil {
		return false, liveMutationReason(err)
	}
	restrict, _, err := ops.detail(ctx, targetID)
	if err != nil {
		return false, liveMutationReason(err)
	}
	contains, err := ops.contains(ctx, userID, pixivsdk.RestrictPublic, tag, targetID)
	if err != nil {
		return false, liveMutationReason(err)
	}
	tags, err := ops.tags(ctx, userID, pixivsdk.RestrictPublic)
	if err != nil {
		return false, liveMutationReason(err)
	}
	if restrict != "" || contains || (tag != "" && slices.Contains(tags, tag)) {
		return false, sdk.MalformedUpstreamResponse
	}
	return true, ""
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

type liveCommentTarget struct {
	contentID int64
}

type commentTargetOps struct {
	family           string
	search           func(context.Context) ([]int64, error)
	comments         func(context.Context, int64) (pixivsdk.CommentPage, error)
	readBack         func(context.Context, int64, int64) (bool, error)
	post             func(context.Context, int64) (int64, error)
	reply            func(context.Context, int64, int64) (int64, error)
	stamp            func(context.Context, int64, int64) (int64, error)
	remove           func(context.Context, int64) error
	rawCommentsPath  string
	rawRepliesPath   string
	rawIDParameter   string
	repliesParameter string
}

func artworkCommentOps(session *pixivLiveSession) commentTargetOps {
	client := session.client
	return commentTargetOps{
		family: "artwork",
		search: func(ctx context.Context) ([]int64, error) {
			page, err := client.SearchArtworks(ctx, pixivsdk.SearchArtworksRequest{Word: "初音ミク", ContentType: pixivsdk.SearchContentTypeIllust})
			if err != nil {
				return nil, err
			}
			return slices.Collect(func(yield func(int64) bool) {
				for _, item := range page.Items {
					if !yield(item.ID) {
						return
					}
				}
			}), nil
		},
		comments: func(ctx context.Context, id int64) (pixivsdk.CommentPage, error) {
			return client.ArtworkComments(ctx, pixivsdk.ArtworkCommentsRequest{ArtworkID: id})
		},
		readBack: func(ctx context.Context, contentID, commentID int64) (bool, error) {
			page, err := client.ArtworkComments(ctx, pixivsdk.ArtworkCommentsRequest{ArtworkID: contentID})
			return slices.ContainsFunc(page.Page.Items, func(item pixivsdk.Comment) bool { return item.ID == commentID }), err
		},
		post: func(ctx context.Context, id int64) (int64, error) {
			result, err := client.PostArtworkComment(ctx, pixivsdk.PostArtworkCommentRequest{ArtworkID: id, Comment: liveCommentBody})
			return result.CommentID, err
		},
		reply: func(ctx context.Context, contentID, parentID int64) (int64, error) {
			result, err := client.ReplyArtworkComment(ctx, pixivsdk.ReplyArtworkCommentRequest{ArtworkID: contentID, ParentCommentID: parentID, Comment: liveCommentBody})
			return result.CommentID, err
		},
		stamp: func(ctx context.Context, contentID, stampID int64) (int64, error) {
			result, err := client.StampArtworkComment(ctx, pixivsdk.StampArtworkCommentRequest{ArtworkID: contentID, StampID: stampID})
			return result.CommentID, err
		},
		remove: func(ctx context.Context, commentID int64) error {
			return client.DeleteArtworkComment(ctx, pixivsdk.DeleteArtworkCommentRequest{CommentID: commentID})
		},
		rawCommentsPath: protocol.AppIllustComments,
		rawRepliesPath:  "/v2/illust/comment/replies",
		rawIDParameter:  "illust_id",
	}
}

func novelCommentOps(session *pixivLiveSession) commentTargetOps {
	client := session.client
	return commentTargetOps{
		family: "novel",
		search: func(ctx context.Context) ([]int64, error) {
			page, err := client.SearchNovels(ctx, pixivsdk.SearchNovelsRequest{Word: "初音ミク"})
			if err != nil {
				return nil, err
			}
			return slices.Collect(func(yield func(int64) bool) {
				for _, item := range page.Items {
					if !yield(item.ID) {
						return
					}
				}
			}), nil
		},
		comments: func(ctx context.Context, id int64) (pixivsdk.CommentPage, error) {
			return client.NovelComments(ctx, pixivsdk.NovelCommentsRequest{NovelID: id})
		},
		readBack: func(ctx context.Context, contentID, commentID int64) (bool, error) {
			page, err := client.NovelComments(ctx, pixivsdk.NovelCommentsRequest{NovelID: contentID})
			return slices.ContainsFunc(page.Page.Items, func(item pixivsdk.Comment) bool { return item.ID == commentID }), err
		},
		post: func(ctx context.Context, id int64) (int64, error) {
			result, err := client.PostNovelComment(ctx, pixivsdk.PostNovelCommentRequest{NovelID: id, Comment: liveCommentBody})
			return result.CommentID, err
		},
		reply: func(ctx context.Context, contentID, parentID int64) (int64, error) {
			result, err := client.ReplyNovelComment(ctx, pixivsdk.ReplyNovelCommentRequest{NovelID: contentID, ParentCommentID: parentID, Comment: liveCommentBody})
			return result.CommentID, err
		},
		stamp: func(ctx context.Context, contentID, stampID int64) (int64, error) {
			result, err := client.StampNovelComment(ctx, pixivsdk.StampNovelCommentRequest{NovelID: contentID, StampID: stampID})
			return result.CommentID, err
		},
		remove: func(ctx context.Context, commentID int64) error {
			return client.DeleteNovelComment(ctx, pixivsdk.DeleteNovelCommentRequest{CommentID: commentID})
		},
		rawCommentsPath: protocol.AppNovelComments,
		rawRepliesPath:  "/v2/novel/comment/replies",
		rawIDParameter:  "novel_id",
	}
}

func readCommentReplies(ctx context.Context, raw *protocol.Client, session *pixivLiveSession, ops commentTargetOps, parentID, commentID int64) (bool, error) {
	ids, err := readRawCommentIDs(ctx, raw, ops.rawRepliesPath, "comment_id", parentID, session.userID)
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

func readCommentRepliesAfterDelete(ctx context.Context, raw *protocol.Client, session *pixivLiveSession, ops commentTargetOps, parentID, commentID int64) (bool, error) {
	present, err := readCommentReplies(ctx, raw, session, ops, parentID, commentID)
	return normalizeReplyCleanupRead(present, err)
}

func runCommentReplyWithCreatedParent(ctx context.Context, session *pixivLiveSession, ops commentTargetOps, target liveCommentTarget) (liveMutationResult, liveMutationResult) {
	parent, err := ops.post(ctx, target.contentID)
	if err != nil {
		result := mutationWriteError(target.contentID, err)
		if result.uncertain || result.status == "correction" {
			result.writes = 1
		}
		return result, blockedMutationResult(target.contentID, fmt.Errorf("%s reply parent setup failed", ops.family))
	}

	parentResult := liveMutationResult{status: "correction", targetID: target.contentID, writes: 1}
	if parent <= 0 {
		parentResult.reason = sdk.MalformedUpstreamResponse
		return parentResult, correctionMutationResult(target.contentID, sdk.MalformedUpstreamResponse)
	}

	present, readErr := ops.readBack(ctx, target.contentID, parent)
	parentResult.readBack = readErr == nil && present
	if readErr != nil {
		parentResult.reason = liveMutationReason(readErr)
	} else if !present {
		parentResult.reason = sdk.MalformedUpstreamResponse
	}

	replyResult := blockedMutationResult(target.contentID, fmt.Errorf("%s reply parent setup was not readable", ops.family))
	if parentResult.readBack {
		replyResult = runLiveCommentMutation(
			target.contentID,
			func() (int64, error) {
				return ops.reply(ctx, target.contentID, parent)
			},
			func(commentID int64) (bool, error) {
				return readCommentReplies(ctx, session.raw, session, ops, parent, commentID)
			},
			func(commentID int64) error { return ops.remove(ctx, commentID) },
			func(commentID int64) (bool, error) {
				return readCommentRepliesAfterDelete(ctx, session.raw, session, ops, parent, commentID)
			},
		)
	}

	if err := ops.remove(ctx, parent); err != nil {
		parentResult.reason = liveMutationReason(err)
		return parentResult, replyResult
	}
	after, afterErr := ops.readBack(ctx, target.contentID, parent)
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

func runCommentMutations(t *testing.T, ctx context.Context, session *pixivLiveSession, ops commentTargetOps, stampID int64) []namedLiveMutationResult {
	t.Helper()
	results := make([]namedLiveMutationResult, 0, 3)
	record := func(name string, result liveMutationResult) {
		results = append(results, namedLiveMutationResult{name: name, result: result})
		recordLiveMutation(t, name, result)
	}

	target, err := findWritableCommentTarget(ctx, ops)
	if err != nil {
		result := commentTargetProbeResult(err)
		record(ops.family+"_comment_text", result)
		record(ops.family+"_comment_stamp", result)
		record(ops.family+"_comment_reply", result)
		return results
	}

	record(ops.family+"_comment_text", runLiveCommentMutation(
		target.contentID,
		func() (int64, error) { return ops.post(ctx, target.contentID) },
		func(commentID int64) (bool, error) {
			return ops.readBack(ctx, target.contentID, commentID)
		},
		func(commentID int64) error { return ops.remove(ctx, commentID) },
		func(commentID int64) (bool, error) {
			return ops.readBack(ctx, target.contentID, commentID)
		},
	))

	stampRead := func(commentID int64) (bool, error) {
		ids, err := readRawCommentIDs(ctx, session.raw, ops.rawCommentsPath, ops.rawIDParameter, target.contentID, session.userID)
		if err != nil {
			return false, err
		}
		_, ok := ids[commentID]
		return ok, nil
	}
	record(ops.family+"_comment_stamp", runLiveCommentMutation(
		target.contentID,
		func() (int64, error) { return ops.stamp(ctx, target.contentID, stampID) },
		stampRead,
		func(commentID int64) error { return ops.remove(ctx, commentID) },
		stampRead,
	))

	parentResult, replyResult := runCommentReplyWithCreatedParent(ctx, session, ops, target)
	recordLiveMutation(t, ops.family+"_comment_reply_parent_setup", parentResult)
	record(ops.family+"_comment_reply", replyResult)
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
	return slices.ContainsFunc(page.Page.Items, func(comment pixivsdk.Comment) bool { return comment.ID > 0 })
}

func findWritableCommentTarget(ctx context.Context, ops commentTargetOps) (liveCommentTarget, error) {
	ids, err := ops.search(ctx)
	if err != nil {
		return liveCommentTarget{}, err
	}
	var lastErr error
	readableCandidates := 0
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		comments, err := ops.comments(ctx, id)
		if err != nil {
			lastErr = err
			continue
		}
		readableCandidates++
		if !commentTargetCanBeProbed(comments) {
			continue
		}
		return liveCommentTarget{contentID: id}, nil
	}
	if readableCandidates == 0 && lastErr != nil {
		return liveCommentTarget{}, lastErr
	}
	return liveCommentTarget{}, commentTargetUnavailable(ops.family)
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
