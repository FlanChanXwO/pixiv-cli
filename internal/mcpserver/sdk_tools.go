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

type userDetailOut struct {
	Records []application.Record `json:"records"`
}

// userDetail 将完整公开 SDK envelope 封装为单条 user record，避免详情 tool 成为
// records 契约的例外。
func (a *App) userDetail(ctx context.Context, _ *mcp.CallToolRequest, in userDetailIn) (*mcp.CallToolResult, userDetailOut, error) {
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
	record, err := application.RecordFromUserDetail(*result)
	if err != nil {
		return a.userDetailError(ctx, err)
	}
	out := userDetailOut{Records: []application.Record{record}}
	return recordResult(out.Records, false, ""), out, nil
}

func (a *App) userDetailError(ctx context.Context, err error) (*mcp.CallToolResult, userDetailOut, error) {
	out := userDetailOut{Records: []application.Record{}}
	return recordResult(out.Records, true, recordErrorMessage(err)), out, nil
}

type recommendedIn struct {
	Kind         string          `json:"kind" jsonschema:"required: all, illust, manga, novel, or user"`
	IllustFilter *illustFilterIn `json:"illust_filter,omitempty"`
	NovelFilter  *novelFilterIn  `json:"novel_filter,omitempty"`
	UserFilter   *userFilterIn   `json:"user_filter,omitempty"`
	pageLimitIn
}
type recommendedOut struct {
	Records    []application.Record     `json:"records"`
	Pagination recommendedPaginationOut `json:"pagination"`
}

// recommendedPaginationOut 分别表达每条推荐流的逻辑分页；SDK opaque cursor 不离开适配层。
// 单类查询只有其对应字段，all 则始终含四个字段。
type recommendedPaginationOut struct {
	Illust *paginationOut `json:"illust,omitempty"`
	Manga  *paginationOut `json:"manga,omitempty"`
	Novel  *paginationOut `json:"novel,omitempty"`
	User   *paginationOut `json:"user,omitempty"`
}

func newRecommendedOut() recommendedOut {
	return recommendedOut{
		Records: []application.Record{},
	}
}

func recommendedPagination(plan mcpListPlan, limit *int, returned int, more bool) *paginationOut {
	value := listPagination(plan, limit, returned, more)
	return &value
}

func (a *App) recommended(ctx context.Context, _ *mcp.CallToolRequest, in recommendedIn) (*mcp.CallToolResult, recommendedOut, error) {
	out := newRecommendedOut()
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err == nil && in.Kind != "all" && in.Kind != "illust" && in.Kind != "manga" && in.Kind != "novel" && in.Kind != "user" {
		err = errors.New("kind must be one of: all, illust, manga, novel, user")
	}
	if err != nil {
		return a.recommendedError(ctx, err)
	}
	ctx, err = withMixedRecordFilters(ctx, in.IllustFilter, in.NovelFilter, in.UserFilter)
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
		records, mapErr := recordsFromIllusts(items)
		if mapErr != nil {
			return a.recommendedError(ctx, mapErr)
		}
		out.Records = append(out.Records, records...)
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
		records, mapErr := recordsFromIllusts(items)
		if mapErr != nil {
			return a.recommendedError(ctx, mapErr)
		}
		out.Records = append(out.Records, records...)
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
		records, mapErr := recordsFromNovels(items)
		if mapErr != nil {
			return a.recommendedError(ctx, mapErr)
		}
		out.Records = append(out.Records, records...)
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
		records, mapErr := recordsFromRecommendedUserPreviews(items)
		if mapErr != nil {
			return a.recommendedError(ctx, mapErr)
		}
		out.Records = append(out.Records, records...)
		out.Pagination.User = recommendedPagination(plan, in.Limit, len(items), more)
	}
	return recordResult(out.Records, false, ""), out, nil
}

func (a *App) recommendedError(ctx context.Context, err error) (*mcp.CallToolResult, recommendedOut, error) {
	out := newRecommendedOut()
	return recordResult(out.Records, true, recordErrorMessage(err)), out, nil
}

type userArtworksIn struct {
	UserID       int64           `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	Type         sdk.IllustType  `json:"type,omitempty" jsonschema:"illust, manga, or ugoira"`
	IllustFilter *illustFilterIn `json:"illust_filter,omitempty"`
	pageLimitIn
}

type illustListOut struct {
	Records    []application.Record `json:"records"`
	Pagination paginationOut        `json:"pagination"`
}

func illustListResult(out illustListOut) *mcp.CallToolResult {
	return recordResult(out.Records, false, "")
}

func (a *App) userArtworks(ctx context.Context, _ *mcp.CallToolRequest, in userArtworksIn) (*mcp.CallToolResult, illustListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	ctx, err = withIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, in.UserID)
	if err != nil {
		return a.illustListError(ctx, err)
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
		return a.illustListError(ctx, err)
	}
	records, err := recordsFromIllusts(items)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	out := illustListOut{Records: records, Pagination: listPagination(plan, in.Limit, len(items), more)}
	return illustListResult(out), out, nil
}

func (a *App) illustListError(ctx context.Context, err error) (*mcp.CallToolResult, illustListOut, error) {
	out := illustListOut{Records: []application.Record{}}
	return recordResult(out.Records, true, recordErrorMessage(err)), out, nil
}

type bookmarksSDKIn struct {
	UserID       int64           `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	Restrict     string          `json:"restrict,omitempty" jsonschema:"public or private"`
	Tag          string          `json:"tag,omitempty"`
	IllustFilter *illustFilterIn `json:"illust_filter,omitempty"`
	pageLimitIn
}

func (a *App) userBookmarks(ctx context.Context, _ *mcp.CallToolRequest, in bookmarksSDKIn) (*mcp.CallToolResult, illustListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	userID := in.UserID
	if err != nil {
		return a.illustListError(ctx, err)
	}
	ctx, err = withIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, userID)
	if err != nil {
		return a.illustListError(ctx, err)
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
		return a.illustListError(ctx, err)
	}
	records, err := recordsFromIllusts(items)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	out := illustListOut{Records: records, Pagination: listPagination(plan, in.Limit, len(items), more)}
	return illustListResult(out), out, nil
}

type followingSDKIn struct {
	UserID     int64         `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	Restrict   string        `json:"restrict,omitempty" jsonschema:"public or private"`
	UserFilter *userFilterIn `json:"user_filter,omitempty"`
	pageLimitIn
}

type userListOut struct {
	Records    []application.Record `json:"records"`
	Pagination paginationOut        `json:"pagination"`
}

func userListResult(out userListOut) *mcp.CallToolResult {
	return recordResult(out.Records, false, "")
}

func (a *App) userFollowing(ctx context.Context, _ *mcp.CallToolRequest, in followingSDKIn) (*mcp.CallToolResult, userListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	userID := in.UserID
	if err != nil {
		return a.userListError(ctx, err)
	}
	ctx, err = withUserFilter(ctx, in.UserFilter)
	if err != nil {
		return a.userListError(ctx, err)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, userID)
	if err != nil {
		return a.userListError(ctx, err)
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
		return a.userListError(ctx, err)
	}
	records, err := recordsFromUserPreviews(items)
	if err != nil {
		return a.userListError(ctx, err)
	}
	out := userListOut{Records: records, Pagination: listPagination(plan, in.Limit, len(items), more)}
	return userListResult(out), out, nil
}

func (a *App) userListError(ctx context.Context, err error) (*mcp.CallToolResult, userListOut, error) {
	out := userListOut{Records: []application.Record{}}
	return recordResult(out.Records, true, recordErrorMessage(err)), out, nil
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
