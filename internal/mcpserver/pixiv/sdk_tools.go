package pixiv

import (
	"context"
	"errors"
	"fmt"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// userDetailIn 只接受明确指定的目标用户，避免把请求误解析为当前认证账号。
type userDetailIn struct {
	UserID int64 `json:"user_id" jsonschema:"required positive Pixiv user ID"`
}

type userDetailOut struct {
	Records []recordpkg.Record `json:"records"`
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
	result, err := client.User(ctx, pixiv.UserRequest{UserID: in.UserID})
	if err != nil {
		return a.userDetailError(ctx, err)
	}
	record, err := recordpkg.RecordFromUserDetail(result)
	if err != nil {
		return a.userDetailError(ctx, err)
	}
	out := userDetailOut{Records: []recordpkg.Record{record}}
	return recordResult(out.Records, false, ""), out, nil
}

func (a *App) userDetailError(ctx context.Context, err error) (*mcp.CallToolResult, userDetailOut, error) {
	out := userDetailOut{Records: []recordpkg.Record{}}
	return recordResult(out.Records, true, recordErrorMessage(err)), out, nil
}

type recommendedIn struct {
	Kind         string          `json:"kind" jsonschema:"required: all, illust, manga, novel, or user"`
	Filter       string          `json:"filter,omitempty" jsonschema:"safe local illustration filter expression"`
	IllustFilter *illustFilterIn `json:"illust_filter,omitempty"`
	NovelFilter  *novelFilterIn  `json:"novel_filter,omitempty"`
	UserFilter   *userFilterIn   `json:"user_filter,omitempty"`
	pageLimitIn
}
type recommendedOut struct {
	Records    []recordpkg.Record       `json:"records"`
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
		Records: []recordpkg.Record{},
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
	ctx, err = withMixedRecordFiltersExpression(ctx, in.IllustFilter, in.Filter, in.NovelFilter, in.UserFilter)
	if err != nil {
		return a.recommendedError(ctx, err)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.recommendedError(ctx, err)
	}
	defer release()
	if in.Kind == "all" || in.Kind == "illust" {
		items, more, fetchErr := collectPages(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			r, e := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: c})
			if e != nil {
				return nil, sdk.Cursor{}, e
			}
			return r.Items, r.Next, nil
		})
		if fetchErr != nil {
			return a.recommendedError(ctx, fetchErr)
		}
		records, mapErr := recordsFromArtworks(items)
		if mapErr != nil {
			return a.recommendedError(ctx, mapErr)
		}
		out.Records = append(out.Records, records...)
		out.Pagination.Illust = recommendedPagination(plan, in.Limit, len(items), more)
	}
	if in.Kind == "all" || in.Kind == "manga" {
		items, more, fetchErr := collectPages(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			r, e := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: c})
			if e != nil {
				return nil, sdk.Cursor{}, e
			}
			return r.Items, r.Next, nil
		})
		if fetchErr != nil {
			return a.recommendedError(ctx, fetchErr)
		}
		records, mapErr := recordsFromArtworks(items)
		if mapErr != nil {
			return a.recommendedError(ctx, mapErr)
		}
		out.Records = append(out.Records, records...)
		out.Pagination.Manga = recommendedPagination(plan, in.Limit, len(items), more)
	}
	if in.Kind == "all" || in.Kind == "novel" {
		items, more, fetchErr := collectPages(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			r, e := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: c})
			if e != nil {
				return nil, sdk.Cursor{}, e
			}
			return r.Items, r.Next, nil
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
		items, more, fetchErr := collectPages(ctx, plan, func(ctx context.Context, c sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
			r, e := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: c})
			if e != nil {
				return nil, sdk.Cursor{}, e
			}
			return r.Items, r.Next, nil
		})
		if fetchErr != nil {
			return a.recommendedError(ctx, fetchErr)
		}
		records, mapErr := recordsFromUserPreviews(items)
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
	Type         string          `json:"type,omitempty" jsonschema:"illust, manga, or ugoira"`
	Filter       string          `json:"filter,omitempty" jsonschema:"safe local illustration filter expression"`
	IllustFilter *illustFilterIn `json:"illust_filter,omitempty"`
	pageLimitIn
}

type illustListOut struct {
	Records    []recordpkg.Record `json:"records"`
	Pagination paginationOut      `json:"pagination"`
}

func illustListResult(out illustListOut) *mcp.CallToolResult {
	return recordResult(out.Records, false, "")
}

func (a *App) userArtworks(ctx context.Context, _ *mcp.CallToolRequest, in userArtworksIn) (*mcp.CallToolResult, illustListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	ctx, err = withIllustFilterExpression(ctx, in.IllustFilter, in.Filter)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, in.UserID)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: userID, Kind: pixiv.ArtworkKind(in.Type), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return a.illustListError(ctx, err)
	}
	records, err := recordsFromArtworks(items)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	out := illustListOut{Records: records, Pagination: listPagination(plan, in.Limit, len(items), more)}
	return illustListResult(out), out, nil
}

func (a *App) illustListError(ctx context.Context, err error) (*mcp.CallToolResult, illustListOut, error) {
	out := illustListOut{Records: []recordpkg.Record{}}
	return recordResult(out.Records, true, recordErrorMessage(err)), out, nil
}

type bookmarksSDKIn struct {
	UserID       int64           `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	Restrict     string          `json:"restrict,omitempty" jsonschema:"public or private"`
	Tag          string          `json:"tag,omitempty"`
	Filter       string          `json:"filter,omitempty" jsonschema:"safe local illustration filter expression"`
	IllustFilter *illustFilterIn `json:"illust_filter,omitempty"`
	pageLimitIn
}

func (a *App) userBookmarks(ctx context.Context, _ *mcp.CallToolRequest, in bookmarksSDKIn) (*mcp.CallToolResult, illustListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	userID := in.UserID
	if err != nil {
		return a.illustListError(ctx, err)
	}
	ctx, err = withIllustFilterExpression(ctx, in.IllustFilter, in.Filter)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, userID)
	if err != nil {
		return a.illustListError(ctx, err)
	}
	defer release()
	request := pixiv.UserArtworkBookmarksRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Tag: in.Tag}
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		request.Cursor = cursor
		result, err := client.UserArtworkBookmarks(ctx, request)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return a.illustListError(ctx, err)
	}
	records, err := recordsFromArtworks(items)
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
	Records    []recordpkg.Record `json:"records"`
	Pagination paginationOut      `json:"pagination"`
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
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.UserFollowing(ctx, pixiv.UserFollowingRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
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
	out := userListOut{Records: []recordpkg.Record{}}
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

func (a *App) runMutation(ctx context.Context, out mutationOut, run func(pixivapp.ClientSet) error) (*mcp.CallToolResult, mutationOut, error) {
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
	return a.runMutation(ctx, out, func(client pixivapp.ClientSet) error {
		return client.AddBookmark(ctx, pixiv.AddBookmarkRequest{ArtworkID: in.IllustID, Restrict: pixiv.Restrict(in.Restrict), Tags: in.Tags})
	})
}

func (a *App) removeBookmark(ctx context.Context, _ *mcp.CallToolRequest, in illustIDSDKIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "remove_bookmark", IllustID: in.IllustID, Text: fmt.Sprintf("Removed bookmark from artwork %d.", in.IllustID)}
	return a.runMutation(ctx, out, func(client pixivapp.ClientSet) error {
		return client.RemoveBookmark(ctx, pixiv.RemoveBookmarkRequest{ArtworkID: in.IllustID})
	})
}

func (a *App) followUser(ctx context.Context, _ *mcp.CallToolRequest, in userMutationIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "follow_user", UserID: in.UserID, Text: fmt.Sprintf("Followed user %d.", in.UserID)}
	return a.runMutation(ctx, out, func(client pixivapp.ClientSet) error {
		return client.FollowUser(ctx, pixiv.FollowUserRequest{UserID: in.UserID, Restrict: pixiv.Restrict(in.Restrict)})
	})
}

func (a *App) unfollowUser(ctx context.Context, _ *mcp.CallToolRequest, in userIDSDKIn) (*mcp.CallToolResult, mutationOut, error) {
	out := mutationOut{Action: "unfollow_user", UserID: in.UserID, Text: fmt.Sprintf("Unfollowed user %d.", in.UserID)}
	return a.runMutation(ctx, out, func(client pixivapp.ClientSet) error {
		return client.UnfollowUser(ctx, pixiv.UnfollowUserRequest{UserID: in.UserID})
	})
}
