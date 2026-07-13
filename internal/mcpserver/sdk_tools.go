package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	legacy "github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	sdk "github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// pageLimitIn 以指针区分未传 limit（兼容旧的“单上游批次”）和显式 limit=0（读取全部）。
// Cursor 始终是 SDK 内部细节，不出现在 MCP tool 输入或输出中。
type pageLimitIn struct {
	Page  *int `json:"page,omitempty" jsonschema:"1-based logical page; requires a positive limit"`
	Limit *int `json:"limit,omitempty" jsonschema:"maximum items; 0 returns all items"`
}

type mcpListPlan struct {
	page     int
	limit    int
	oneBatch bool
	skip     int
}

type paginationOut struct {
	Page     int  `json:"page"`
	Limit    *int `json:"limit"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"has_more"`
	NextPage *int `json:"next_page"`
}

func parseMCPListPlan(input pageLimitIn) (mcpListPlan, error) {
	if input.Page != nil && *input.Page <= 0 {
		return mcpListPlan{}, errors.New("page must be a positive integer")
	}
	if input.Limit != nil && *input.Limit < 0 {
		return mcpListPlan{}, errors.New("limit must be zero or a positive integer")
	}
	if input.Page != nil {
		if input.Limit == nil || *input.Limit <= 0 {
			return mcpListPlan{}, errors.New("page requires limit to be a positive integer")
		}
		if *input.Page-1 > math.MaxInt / *input.Limit {
			return mcpListPlan{}, errors.New("page and limit overflow the logical result offset")
		}
		return mcpListPlan{page: *input.Page, limit: *input.Limit, skip: (*input.Page - 1) * *input.Limit}, nil
	}
	if input.Limit == nil {
		return mcpListPlan{page: 1, limit: -1, oneBatch: true}, nil
	}
	return mcpListPlan{page: 1, limit: *input.Limit}, nil
}

func listPagination(plan mcpListPlan, limit *int, returned int, hasMore bool) paginationOut {
	out := paginationOut{Page: plan.page, Limit: limit, Returned: returned, HasMore: hasMore}
	if hasMore && limit != nil && *limit > 0 && plan.page < math.MaxInt {
		next := plan.page + 1
		out.NextPage = &next
	}
	return out
}

// collectPages 与 CLI 同样只受 caller limit、上游 cursor 和 context 约束。seen 不设产品上限，
// 仅防御上游错误重复 cursor 导致无限请求。
func collectPages[T any](ctx context.Context, plan mcpListPlan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, bool, error) {
	cursor := sdk.Cursor("")
	seen := make(map[sdk.Cursor]struct{})
	skip := plan.skip
	seekingOffset := skip > 0
	itemsOut := make([]T, 0)
	for {
		if _, exists := seen[cursor]; exists {
			return nil, false, fmt.Errorf("pagination cursor repeated: %q", cursor)
		}
		seen[cursor] = struct{}{}
		items, next, err := fetch(ctx, cursor)
		if err != nil {
			return nil, false, err
		}
		if skip > 0 {
			if skip >= len(items) {
				skip -= len(items)
				items = nil
			} else {
				items = items[skip:]
				skip = 0
			}
		}
		if seekingOffset && skip == 0 && len(items) > 0 {
			seekingOffset = false
		}
		if plan.limit > 0 && len(items) > plan.limit-len(itemsOut) {
			items = items[:plan.limit-len(itemsOut)]
			// 当前批次还有未输出元素，逻辑页后仍有内容。
			return append(itemsOut, items...), true, nil
		}
		itemsOut = append(itemsOut, items...)
		if (plan.oneBatch && !seekingOffset) || (plan.limit > 0 && len(itemsOut) >= plan.limit) {
			return itemsOut, next != "", nil
		}
		if next == "" {
			return itemsOut, false, nil
		}
		cursor = next
	}
}

func (a *App) openSDKOperation(ctx context.Context) (application.SDKClient, func(), error) {
	if a.sdk.NewClient == nil {
		return nil, nil, errors.New("pixiv sdk is not configured")
	}
	a.sdkMu.Lock()
	client, err := a.sdk.OpenOperation(ctx, a.sdkRequest)
	if err != nil {
		a.sdkMu.Unlock()
		return nil, nil, err
	}
	return client, a.releaseSDKOperation, nil
}

func (a *App) currentSDKUser(ctx context.Context) (application.SDKClient, int64, func(), error) {
	if a.sdk.NewClient == nil {
		return nil, 0, nil, errors.New("pixiv sdk is not configured")
	}
	a.sdkMu.Lock()
	client, err := a.sdk.OpenOperation(ctx, a.sdkRequest)
	if err != nil {
		a.sdkMu.Unlock()
		return nil, 0, nil, err
	}
	userID, err := client.CurrentUserID(ctx)
	if err != nil {
		a.sdkMu.Unlock()
		return nil, 0, nil, err
	}
	return client, userID, a.releaseSDKOperation, nil
}

func (a *App) releaseSDKOperation() {
	// SDK 已在 selected store 内写入旋转 token；同步 legacy Source 仅为让旧 tool
	// 后续 refresh 继续使用同一权威状态。同步失败不改变已完成的 SDK 调用结果。
	a.syncSourceTokenFromStore()
	a.sdkMu.Unlock()
}

func (a *App) syncSourceTokenFromStore() {
	if a.api == nil {
		return
	}
	path := a.sdkRequest.AuthFilePath
	if path == "" {
		var err error
		path, err = auth.AuthFilePath()
		if err != nil {
			return
		}
	}
	store, err := auth.LoadAuthStore(path)
	if err != nil {
		return
	}
	userID := a.sdkRequest.UserID
	if userID == 0 {
		userID = store.DefaultUserID
	}
	if _, account, ok := store.Get(userID); ok {
		a.api.SetRefreshToken(account.RefreshToken)
	}
}

// persistSourceAuth 把 legacy Source 已验证的认证结果同步到公共 SDK 的本地 store。
// 只在 NewWithSDK 下调用；SDK 后续 OpenDefault 快照会从该 store 取得并安全持久化 rotation。
func (a *App) persistSourceAuth() error {
	if a.sdk.NewClient == nil {
		return nil
	}
	if a.api == nil || a.api.UserID() <= 0 || a.api.RefreshTokenValue() == "" {
		return errors.New("authenticated source did not provide account state")
	}
	path := a.sdkRequest.AuthFilePath
	if path == "" {
		var err error
		path, err = auth.AuthFilePath()
		if err != nil {
			return err
		}
	}
	store, err := auth.LoadAuthStore(path)
	if err != nil {
		return err
	}
	store.Upsert(auth.Account{UserID: a.api.UserID(), Username: a.api.UserName(), RefreshToken: a.api.RefreshTokenValue()})
	store.DefaultUserID = a.api.UserID()
	return auth.SaveAuthStore(path, store)
}

func resolveSDKUser(ctx context.Context, app *App, userID int64) (application.SDKClient, int64, func(), error) {
	if userID != 0 {
		client, release, err := app.openSDKOperation(ctx)
		return client, userID, release, err
	}
	return app.currentSDKUser(ctx)
}

type userArtworksIn struct {
	UserID int64          `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	Type   sdk.IllustType `json:"type,omitempty" jsonschema:"illust, manga, or ugoira"`
	pageLimitIn
}

type illustListOut struct {
	UserID     int64         `json:"user_id"`
	Items      []sdk.Illust  `json:"items"`
	Pagination paginationOut `json:"pagination"`
	Text       string        `json:"text"`
}

func illustListResult(out illustListOut) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

func (a *App) userArtworks(ctx context.Context, _ *mcp.CallToolRequest, in userArtworksIn) (*mcp.CallToolResult, illustListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.illustListError(in.UserID, err)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, in.UserID)
	if err != nil {
		return a.illustListError(in.UserID, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.UserArtworks(ctx, sdk.UserArtworksRequest{UserID: userID, Type: in.Type, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return a.illustListError(userID, err)
	}
	text := fmt.Sprintf("找到用户 %d 的 %d 个作品:\n\n%s", userID, len(items), formatSDKIllusts(items))
	if len(items) == 0 {
		text = fmt.Sprintf("找不到用户 %d 的作品。", userID)
	}
	out := illustListOut{UserID: userID, Items: items, Pagination: listPagination(plan, in.Limit, len(items), more), Text: text}
	return illustListResult(out), out, nil
}

func (a *App) illustListError(userID int64, err error) (*mcp.CallToolResult, illustListOut, error) {
	out := illustListOut{UserID: userID, Items: []sdk.Illust{}, Text: "错误: " + err.Error()}
	return illustListResult(out), out, nil
}

type bookmarksSDKIn struct {
	UserID        int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	UserIDToCheck int64  `json:"user_id_to_check,omitempty" jsonschema:"legacy alias for user_id"`
	Restrict      string `json:"restrict,omitempty" jsonschema:"public or private"`
	Tag           string `json:"tag,omitempty"`
	MaxBookmarkID int64  `json:"max_bookmark_id,omitempty" jsonschema:"deprecated legacy continuation parameter"`
	pageLimitIn
}

func (in bookmarksSDKIn) resolvedUserID() int64 {
	if in.UserID != 0 {
		return in.UserID
	}
	return in.UserIDToCheck
}

func (a *App) userBookmarks(ctx context.Context, _ *mcp.CallToolRequest, in bookmarksSDKIn) (*mcp.CallToolResult, illustListOut, error) {
	// New 保持旧构造器的完整行为：嵌入方尚未迁移到 SDK 时，既有 tool 名和所有
	// legacy 参数仍由 Source 处理。生产 MCP 通过 NewWithSDK 进入下方公共 SDK 路径。
	if a.sdk.NewClient == nil {
		return a.legacyBookmarks(ctx, in)
	}
	plan, err := parseMCPListPlan(in.pageLimitIn)
	userID := in.resolvedUserID()
	if err != nil {
		return a.illustListError(userID, err)
	}
	if in.MaxBookmarkID != 0 {
		if in.Page != nil || in.Limit != nil {
			return a.illustListError(userID, errors.New("max_bookmark_id cannot be combined with page or limit"))
		}
		return a.legacyBookmarks(ctx, in)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, userID)
	if err != nil {
		return a.illustListError(userID, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.UserBookmarks(ctx, sdk.UserBookmarksRequest{UserID: userID, Restrict: sdk.Restrict(in.Restrict), Tag: in.Tag, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return a.illustListError(userID, err)
	}
	text := fmt.Sprintf("找到用户 %d 的 %d 个收藏:\n\n%s", userID, len(items), formatSDKIllusts(items))
	if len(items) == 0 {
		text = fmt.Sprintf("找不到用户 %d 的收藏。", userID)
	}
	out := illustListOut{UserID: userID, Items: items, Pagination: listPagination(plan, in.Limit, len(items), more), Text: text}
	return illustListResult(out), out, nil
}

// legacyBookmarks 只处理旧 MCP 已公开的 max_bookmark_id continuation。公共 SDK 有意隐藏
// 上游 ID 并以 opaque cursor 取代它，因此新 page/limit 调用不会经过这里。
func (a *App) legacyBookmarks(ctx context.Context, in bookmarksSDKIn) (*mcp.CallToolResult, illustListOut, error) {
	if err := a.ensureAuth(ctx); err != nil {
		return a.legacyIllustListError(in.resolvedUserID(), err)
	}
	userID := in.resolvedUserID()
	if userID == 0 {
		userID = a.api.UserID()
	}
	if userID == 0 {
		return a.legacyIllustListError(0, errors.New("错误: 查询自己的收藏时，需要先认证以获取用户ID。"))
	}
	restrict := in.Restrict
	if restrict == "" {
		restrict = string(sdk.RestrictPublic)
	}
	result, err := a.api.UserBookmarks(ctx, userID, restrict, in.Tag, in.MaxBookmarkID)
	if err != nil {
		return a.legacyIllustListError(userID, err)
	}
	items, err := normalizeLegacyIllusts(result.Illusts)
	if err != nil {
		return a.legacyIllustListError(userID, err)
	}
	text := fmt.Sprintf("找到用户 %d 的 %d 个收藏:\n\n%s", userID, len(items), formatSDKIllusts(items))
	if len(items) == 0 {
		text = fmt.Sprintf("找不到用户 %d 的收藏。", userID)
	}
	out := illustListOut{UserID: userID, Items: items, Pagination: paginationOut{Page: 1, Returned: len(items)}, Text: text}
	return illustListResult(out), out, nil
}

func (a *App) legacyIllustListError(userID int64, err error) (*mcp.CallToolResult, illustListOut, error) {
	out := illustListOut{UserID: userID, Items: []sdk.Illust{}, Text: err.Error()}
	return illustListResult(out), out, nil
}

func normalizeLegacyIllusts(items []legacy.Illust) ([]sdk.Illust, error) {
	encoded, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	var normalized []sdk.Illust
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	if normalized == nil {
		normalized = []sdk.Illust{}
	}
	return normalized, nil
}

type followingSDKIn struct {
	UserID        int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	UserIDToCheck int64  `json:"user_id_to_check,omitempty" jsonschema:"legacy alias for user_id"`
	Restrict      string `json:"restrict,omitempty" jsonschema:"public or private"`
	Offset        *int   `json:"offset,omitempty" jsonschema:"deprecated legacy logical result offset"`
	pageLimitIn
}

func (in followingSDKIn) resolvedUserID() int64 {
	if in.UserID != 0 {
		return in.UserID
	}
	return in.UserIDToCheck
}

type userListOut struct {
	UserID     int64             `json:"user_id"`
	Items      []sdk.UserPreview `json:"items"`
	Pagination paginationOut     `json:"pagination"`
	Text       string            `json:"text"`
}

func userListResult(out userListOut) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

func (a *App) userFollowing(ctx context.Context, _ *mcp.CallToolRequest, in followingSDKIn) (*mcp.CallToolResult, userListOut, error) {
	if a.sdk.NewClient == nil {
		return a.legacyFollowing(ctx, in)
	}
	plan, err := parseMCPListPlan(in.pageLimitIn)
	userID := in.resolvedUserID()
	if err != nil {
		return a.userListError(userID, err)
	}
	if in.Offset != nil {
		if in.Page != nil {
			return a.userListError(userID, errors.New("page and deprecated offset cannot be used together"))
		}
		if *in.Offset < 0 {
			return a.userListError(userID, errors.New("offset must be zero or a positive integer"))
		}
		plan.skip = *in.Offset
		plan.oneBatch = in.Limit == nil
	}
	client, userID, release, err := resolveSDKUser(ctx, a, userID)
	if err != nil {
		return a.userListError(userID, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.UserPreview, sdk.Cursor, error) {
		result, err := client.UserFollowing(ctx, sdk.UserFollowingRequest{UserID: userID, Restrict: sdk.Restrict(in.Restrict), Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.UserPreviews, result.NextCursor, nil
	})
	if err != nil {
		return a.userListError(userID, err)
	}
	text := fmt.Sprintf("用户 %d 关注了 %d 位用户:\n\n%s", userID, len(items), formatSDKUsers(items))
	if len(items) == 0 {
		text = fmt.Sprintf("用户 %d 没有关注任何人。", userID)
	}
	out := userListOut{UserID: userID, Items: items, Pagination: listPagination(plan, in.Limit, len(items), more), Text: text}
	return userListResult(out), out, nil
}

func (a *App) legacyFollowing(ctx context.Context, in followingSDKIn) (*mcp.CallToolResult, userListOut, error) {
	if err := a.ensureAuth(ctx); err != nil {
		return a.legacyUserListError(in.resolvedUserID(), err)
	}
	userID := in.resolvedUserID()
	if userID == 0 {
		userID = a.api.UserID()
	}
	if userID == 0 {
		return a.legacyUserListError(0, errors.New("错误: 查询自己的关注列表时，需要先认证以获取用户ID。"))
	}
	restrict := in.Restrict
	if restrict == "" {
		restrict = string(sdk.RestrictPublic)
	}
	offset := 0
	if in.Offset != nil {
		offset = *in.Offset
	}
	result, err := a.api.UserFollowing(ctx, userID, restrict, offset)
	if err != nil {
		return a.legacyUserListError(userID, err)
	}
	items, err := normalizeLegacyUsers(result.UserPreviews)
	if err != nil {
		return a.legacyUserListError(userID, err)
	}
	text := fmt.Sprintf("用户 %d 关注了 %d 位用户:\n\n%s", userID, len(items), formatSDKUsers(items))
	if len(items) == 0 {
		text = fmt.Sprintf("用户 %d 没有关注任何人。", userID)
	}
	out := userListOut{UserID: userID, Items: items, Pagination: paginationOut{Page: 1, Returned: len(items)}, Text: text}
	return userListResult(out), out, nil
}

func normalizeLegacyUsers(items []legacy.UserPreview) ([]sdk.UserPreview, error) {
	encoded, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	var normalized []sdk.UserPreview
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	if normalized == nil {
		normalized = []sdk.UserPreview{}
	}
	return normalized, nil
}

func (a *App) legacyUserListError(userID int64, err error) (*mcp.CallToolResult, userListOut, error) {
	out := userListOut{UserID: userID, Items: []sdk.UserPreview{}, Text: err.Error()}
	return userListResult(out), out, nil
}

func (a *App) userListError(userID int64, err error) (*mcp.CallToolResult, userListOut, error) {
	out := userListOut{UserID: userID, Items: []sdk.UserPreview{}, Text: "错误: " + err.Error()}
	return userListResult(out), out, nil
}

type bookmarkMutationIn struct {
	IllustID int64    `json:"illust_id" jsonschema:"artwork ID"`
	Restrict string   `json:"restrict,omitempty" jsonschema:"public or private"`
	Tags     []string `json:"tags,omitempty" jsonschema:"bookmark tags"`
}

type illustIDSDKIn struct {
	IllustID int64 `json:"illust_id" jsonschema:"artwork ID"`
}

type userMutationIn struct {
	UserID   int64  `json:"user_id" jsonschema:"user ID"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
}

type userIDSDKIn struct {
	UserID int64 `json:"user_id" jsonschema:"user ID"`
}

type mutationOut struct {
	Success  bool   `json:"success"`
	Action   string `json:"action"`
	IllustID int64  `json:"illust_id,omitempty"`
	UserID   int64  `json:"user_id,omitempty"`
	Text     string `json:"text"`
}

func mutationResult(out mutationOut) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

func (a *App) runMutation(ctx context.Context, out mutationOut, run func(application.SDKClient) error) (*mcp.CallToolResult, mutationOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err == nil {
		defer release()
		err = run(client)
	}
	if err != nil {
		out.Text = "错误: " + err.Error()
		return mutationResult(out), out, nil
	}
	out.Success = true
	return mutationResult(out), out, nil
}

func (a *App) addBookmark(ctx context.Context, _ *mcp.CallToolRequest, in bookmarkMutationIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "add_bookmark", IllustID: in.IllustID, Text: fmt.Sprintf("已收藏作品 %d。", in.IllustID)}
	return a.runMutation(ctx, out, func(client application.SDKClient) error {
		return client.AddBookmark(ctx, sdk.AddBookmarkRequest{IllustID: in.IllustID, Restrict: sdk.Restrict(in.Restrict), Tags: in.Tags})
	})
}

func (a *App) removeBookmark(ctx context.Context, _ *mcp.CallToolRequest, in illustIDSDKIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "remove_bookmark", IllustID: in.IllustID, Text: fmt.Sprintf("已取消收藏作品 %d。", in.IllustID)}
	return a.runMutation(ctx, out, func(client application.SDKClient) error {
		return client.RemoveBookmark(ctx, sdk.RemoveBookmarkRequest{IllustID: in.IllustID})
	})
}

func (a *App) followUser(ctx context.Context, _ *mcp.CallToolRequest, in userMutationIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "follow_user", UserID: in.UserID, Text: fmt.Sprintf("已关注用户 %d。", in.UserID)}
	return a.runMutation(ctx, out, func(client application.SDKClient) error {
		return client.FollowUser(ctx, sdk.FollowUserRequest{UserID: in.UserID, Restrict: sdk.Restrict(in.Restrict)})
	})
}

func (a *App) unfollowUser(ctx context.Context, _ *mcp.CallToolRequest, in userIDSDKIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "unfollow_user", UserID: in.UserID, Text: fmt.Sprintf("已取消关注用户 %d。", in.UserID)}
	return a.runMutation(ctx, out, func(client application.SDKClient) error {
		return client.UnfollowUser(ctx, sdk.UnfollowUserRequest{UserID: in.UserID})
	})
}

// formatSDKIllusts/formatSDKUsers 延续旧 MCP 中文文本结果，同时 structured content 提供
// 面向调用方的规范化 SDK 模型。
func formatSDKIllusts(illusts []sdk.Illust) string {
	lines := make([]string, 0, len(illusts))
	for _, illust := range illusts {
		tags := make([]string, 0, min(len(illust.Tags), 5))
		for _, tag := range illust.Tags {
			if len(tags) == 5 {
				break
			}
			tags = append(tags, tag.Name)
		}
		lines = append(lines, fmt.Sprintf("ID: %d - %q\n  作者: %s (ID: %d)\n  类型: %s\n  标签: %s\n  收藏数: %d, 浏览数: %d",
			illust.ID, illust.Title, illust.User.Name, illust.User.ID, illust.Type, strings.Join(tags, ", "), illust.TotalBookmarks, illust.TotalView))
	}
	return strings.Join(lines, "\n\n")
}

func formatSDKUsers(users []sdk.UserPreview) string {
	lines := make([]string, 0, len(users))
	for _, preview := range users {
		user := preview.User
		followed := "未关注"
		if user.IsFollowed {
			followed = "已关注"
		}
		comment := user.Comment
		if comment == "" {
			comment = "无"
		}
		lines = append(lines, fmt.Sprintf("用户ID: %d - %s (@%s)\n  关注状态: %s\n  简介: %s", user.ID, user.Name, user.Account, followed, comment))
	}
	return strings.Join(lines, "\n\n")
}
