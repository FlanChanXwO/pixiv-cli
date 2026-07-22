package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// userDetailIn 只接受明确指定的目标用户，避免把请求误解析为当前认证账号。
type userDetailIn struct {
	UserID int64 `json:"user_id" jsonschema:"required positive Pixiv user ID"`
}

// userDetail 直接返回稳定的公开 SDK envelope；MCP 不重新映射或裁剪详情字段。
func (a *App) userDetail(ctx context.Context, _ *mcp.CallToolRequest, in userDetailIn) (*mcp.CallToolResult, sdk.UserDetailResult, error) {
	if in.UserID <= 0 {
		return a.userDetailError(ctx, fmt.Errorf("user_id must be a positive integer"))
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.userDetailError(ctx, err)
	}
	defer release()
	result, err := client.UserDetail(ctx, sdk.UserDetailRequest{UserID: in.UserID})
	if err != nil {
		return a.userDetailError(ctx, err)
	}
	if result == nil {
		return a.userDetailError(ctx, errors.New("pixiv sdk returned an empty user detail result"))
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved details for user %d.", in.UserID)}}}, *result, nil
}

func (a *App) userDetailError(ctx context.Context, err error) (*mcp.CallToolResult, sdk.UserDetailResult, error) {
	recordToolError(ctx, err)
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}},
	}, sdk.UserDetailResult{}, nil
}

type recommendedIn struct {
	Kind string `json:"kind" jsonschema:"required: all, illust, manga, novel, or user"`
	pageLimitIn
}
type recommendedOut struct {
	Kind         string                       `json:"kind"`
	Illusts      []sdk.Illust                 `json:"illusts"`
	Manga        []sdk.Illust                 `json:"manga"`
	Novels       []sdk.Novel                  `json:"novels"`
	UserPreviews []sdk.RecommendedUserPreview `json:"user_previews"`
	Pagination   recommendedPaginationOut     `json:"pagination"`
}

// recommendedPaginationOut 分别表达每条推荐流的逻辑分页；SDK opaque cursor 不离开适配层。
// 单类查询只有其对应字段，all 则始终含四个字段。
type recommendedPaginationOut struct {
	Illust *paginationOut `json:"illust,omitempty"`
	Manga  *paginationOut `json:"manga,omitempty"`
	Novel  *paginationOut `json:"novel,omitempty"`
	User   *paginationOut `json:"user,omitempty"`
}

func newRecommendedOut(kind string) recommendedOut {
	return recommendedOut{
		Kind: kind, Illusts: []sdk.Illust{}, Manga: []sdk.Illust{}, Novels: []sdk.Novel{}, UserPreviews: []sdk.RecommendedUserPreview{},
	}
}

func recommendedPagination(plan mcpListPlan, limit *int, returned int, more bool) *paginationOut {
	value := listPagination(plan, limit, returned, more)
	return &value
}

func (a *App) recommended(ctx context.Context, _ *mcp.CallToolRequest, in recommendedIn) (*mcp.CallToolResult, recommendedOut, error) {
	out := newRecommendedOut(in.Kind)
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err == nil && in.Kind != "all" && in.Kind != "illust" && in.Kind != "manga" && in.Kind != "novel" && in.Kind != "user" {
		err = errors.New("kind must be one of: all, illust, manga, novel, user")
	}
	if err != nil {
		return a.recommendedError(ctx, err)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.recommendedError(ctx, err)
	}
	defer release()
	if in.Kind == "all" || in.Kind == "illust" {
		items, more, fetchErr := collectPages(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
			r, e := client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: c})
			if e != nil {
				return nil, "", e
			}
			return r.Illusts, r.NextCursor, nil
		})
		if fetchErr != nil {
			return a.recommendedError(ctx, fetchErr)
		}
		out.Illusts = normalizeIllusts(items)
		out.Pagination.Illust = recommendedPagination(plan, in.Limit, len(items), more)
	}
	if in.Kind == "all" || in.Kind == "manga" {
		items, more, fetchErr := collectPages(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
			r, e := client.MangaRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: c})
			if e != nil {
				return nil, "", e
			}
			return r.Illusts, r.NextCursor, nil
		})
		if fetchErr != nil {
			return a.recommendedError(ctx, fetchErr)
		}
		out.Manga = normalizeIllusts(items)
		out.Pagination.Manga = recommendedPagination(plan, in.Limit, len(items), more)
	}
	if in.Kind == "all" || in.Kind == "novel" {
		items, more, fetchErr := collectPages(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error) {
			r, e := client.NovelRecommended(ctx, sdk.NovelRecommendedRequest{Cursor: c})
			if e != nil {
				return nil, "", e
			}
			return r.Novels, r.NextCursor, nil
		})
		if fetchErr != nil {
			return a.recommendedError(ctx, fetchErr)
		}
		out.Novels = normalizeRecommendedNovels(items)
		out.Pagination.Novel = recommendedPagination(plan, in.Limit, len(items), more)
	}
	if in.Kind == "all" || in.Kind == "user" {
		items, more, fetchErr := collectPages(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]sdk.RecommendedUserPreview, sdk.Cursor, error) {
			r, e := client.UserRecommended(ctx, sdk.UserRecommendedRequest{Cursor: c})
			if e != nil {
				return nil, "", e
			}
			return r.UserPreviews, r.NextCursor, nil
		})
		if fetchErr != nil {
			return a.recommendedError(ctx, fetchErr)
		}
		out.UserPreviews = normalizeRecommendedUserPreviews(items)
		out.Pagination.User = recommendedPagination(plan, in.Limit, len(items), more)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved %s recommendations.", in.Kind)}}}, out, nil
}

// normalizeRecommendedUserPreviews 只在 MCP 输出边界把可选作品预览归一为空数组，
// 使其满足 tool schema；公共 SDK 仍保留 nil 与空切片的原始 Go 语义。
func normalizeRecommendedUserPreviews(items []sdk.RecommendedUserPreview) []sdk.RecommendedUserPreview {
	result := make([]sdk.RecommendedUserPreview, len(items))
	copy(result, items)
	for index := range result {
		result[index].Illusts = normalizeIllusts(result[index].Illusts)
		if result[index].Novels == nil {
			result[index].Novels = []sdk.Novel{}
		}
		result[index].Novels = normalizeRecommendedNovels(result[index].Novels)
	}
	return result
}

// normalizeRecommendedNovels 保证 MCP schema 的 tags 字段始终编码为数组。
func normalizeRecommendedNovels(items []sdk.Novel) []sdk.Novel {
	result := make([]sdk.Novel, len(items))
	copy(result, items)
	for index := range result {
		if result[index].Tags == nil {
			result[index].Tags = []sdk.Tag{}
		}
	}
	return result
}

// normalizeIllusts 只在 MCP structured output 边界把缺失的工具列表编码为空数组。
// 复制作品切片可避免适配层改写 public SDK 返回值；非空工具名保持原值和上游顺序。
func normalizeIllusts(items []sdk.Illust) []sdk.Illust {
	result := make([]sdk.Illust, len(items))
	copy(result, items)
	for index := range result {
		if result[index].URL == "" && result[index].ID > 0 {
			result[index].URL = fmt.Sprintf("https://www.pixiv.net/artworks/%d", result[index].ID)
		}
		if result[index].Tools == nil {
			result[index].Tools = []string{}
		}
		if result[index].Tags == nil {
			result[index].Tags = []sdk.Tag{}
		}
		if result[index].MetaPages == nil {
			result[index].MetaPages = []sdk.MetaPage{}
		}
	}
	return result
}

func (a *App) recommendedError(ctx context.Context, err error) (*mcp.CallToolResult, recommendedOut, error) {
	recordToolError(ctx, err)
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + err.Error()}}}, newRecommendedOut(""), nil
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
		return a.illustListError(ctx, in.UserID, err)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, in.UserID)
	if err != nil {
		return a.illustListError(ctx, in.UserID, err)
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
		return a.illustListError(ctx, userID, err)
	}
	text := fmt.Sprintf("Found %d artworks by user %d:\n\n%s", len(items), userID, formatSDKIllusts(items))
	if len(items) == 0 {
		text = fmt.Sprintf("No artworks found for user %d.", userID)
	}
	out := illustListOut{UserID: userID, Items: normalizeIllusts(items), Pagination: listPagination(plan, in.Limit, len(items), more), Text: text}
	return illustListResult(out), out, nil
}

func (a *App) illustListError(ctx context.Context, userID int64, err error) (*mcp.CallToolResult, illustListOut, error) {
	recordToolError(ctx, err)
	out := illustListOut{UserID: userID, Items: []sdk.Illust{}, Text: "Error: " + err.Error()}
	result := illustListResult(out)
	result.IsError = true
	return result, out, nil
}

type bookmarksSDKIn struct {
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	Tag      string `json:"tag,omitempty"`
	pageLimitIn
}

func (a *App) userBookmarks(ctx context.Context, _ *mcp.CallToolRequest, in bookmarksSDKIn) (*mcp.CallToolResult, illustListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	userID := in.UserID
	if err != nil {
		return a.illustListError(ctx, userID, err)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, userID)
	if err != nil {
		return a.illustListError(ctx, userID, err)
	}
	defer release()
	request := sdk.UserBookmarksRequest{UserID: userID, Restrict: sdk.Restrict(in.Restrict), Tag: in.Tag}
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		request.Cursor = cursor
		result, err := client.UserBookmarks(ctx, request)
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return a.illustListError(ctx, userID, err)
	}
	text := fmt.Sprintf("Found %d bookmarks for user %d:\n\n%s", len(items), userID, formatSDKIllusts(items))
	if len(items) == 0 {
		text = fmt.Sprintf("No bookmarks found for user %d.", userID)
	}
	out := illustListOut{UserID: userID, Items: normalizeIllusts(items), Pagination: listPagination(plan, in.Limit, len(items), more), Text: text}
	return illustListResult(out), out, nil
}

type followingSDKIn struct {
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	pageLimitIn
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
	plan, err := parseMCPListPlan(in.pageLimitIn)
	userID := in.UserID
	if err != nil {
		return a.userListError(ctx, userID, err)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, userID)
	if err != nil {
		return a.userListError(ctx, userID, err)
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
		return a.userListError(ctx, userID, err)
	}
	text := fmt.Sprintf("User %d follows %d users:\n\n%s", userID, len(items), formatSDKUsers(items))
	if len(items) == 0 {
		text = fmt.Sprintf("User %d does not follow anyone.", userID)
	}
	out := userListOut{UserID: userID, Items: items, Pagination: listPagination(plan, in.Limit, len(items), more), Text: text}
	return userListResult(out), out, nil
}

func (a *App) userListError(ctx context.Context, userID int64, err error) (*mcp.CallToolResult, userListOut, error) {
	recordToolError(ctx, err)
	out := userListOut{UserID: userID, Items: []sdk.UserPreview{}, Text: "Error: " + err.Error()}
	result := userListResult(out)
	result.IsError = true
	return result, out, nil
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
	return &mcp.CallToolResult{IsError: !out.Success, Content: []mcp.Content{&mcp.TextContent{Text: out.Text}}}
}

func (a *App) runMutation(ctx context.Context, out mutationOut, run func(application.SDKClient) error) (*mcp.CallToolResult, mutationOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err == nil {
		defer release()
		err = run(client)
	}
	if err != nil {
		recordToolError(ctx, err)
		out.Text = "Error: " + err.Error()
		return mutationResult(out), out, nil
	}
	out.Success = true
	return mutationResult(out), out, nil
}

func (a *App) addBookmark(ctx context.Context, _ *mcp.CallToolRequest, in bookmarkMutationIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "add_bookmark", IllustID: in.IllustID, Text: fmt.Sprintf("Bookmarked artwork %d.", in.IllustID)}
	return a.runMutation(ctx, out, func(client application.SDKClient) error {
		return client.AddBookmark(ctx, sdk.AddBookmarkRequest{IllustID: in.IllustID, Restrict: sdk.Restrict(in.Restrict), Tags: in.Tags})
	})
}

func (a *App) removeBookmark(ctx context.Context, _ *mcp.CallToolRequest, in illustIDSDKIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "remove_bookmark", IllustID: in.IllustID, Text: fmt.Sprintf("Removed bookmark from artwork %d.", in.IllustID)}
	return a.runMutation(ctx, out, func(client application.SDKClient) error {
		return client.RemoveBookmark(ctx, sdk.RemoveBookmarkRequest{IllustID: in.IllustID})
	})
}

func (a *App) followUser(ctx context.Context, _ *mcp.CallToolRequest, in userMutationIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "follow_user", UserID: in.UserID, Text: fmt.Sprintf("Followed user %d.", in.UserID)}
	return a.runMutation(ctx, out, func(client application.SDKClient) error {
		return client.FollowUser(ctx, sdk.FollowUserRequest{UserID: in.UserID, Restrict: sdk.Restrict(in.Restrict)})
	})
}

func (a *App) unfollowUser(ctx context.Context, _ *mcp.CallToolRequest, in userIDSDKIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "unfollow_user", UserID: in.UserID, Text: fmt.Sprintf("Unfollowed user %d.", in.UserID)}
	return a.runMutation(ctx, out, func(client application.SDKClient) error {
		return client.UnfollowUser(ctx, sdk.UnfollowUserRequest{UserID: in.UserID})
	})
}
