package pixiv

import (
	"context"
	"errors"

	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// novelListOut 为不绑定特定用户的小说列表提供统一 records 输出。
// 不把 SDK opaque cursor 暴露给调用者，分页只使用 MCP 逻辑 page/limit。
type novelListOut struct {
	Records    []recordpkg.Record `json:"records"`
	Pagination paginationOut      `json:"pagination"`
}

func novelListResult(out novelListOut, isError bool) *mcp.CallToolResult {
	return recordResult(out.Records, isError, "")
}

func (a *App) novelListError(ctx context.Context, err error) (*mcp.CallToolResult, novelListOut, error) {
	out := novelListOut{Records: []recordpkg.Record{}, Pagination: paginationOut{Page: 1}}
	return recordResult(out.Records, true, recordErrorMessage(err)), out, nil
}

type novelFollowIn struct {
	Restrict    string         `json:"restrict,omitempty" jsonschema:"public or private"`
	NovelFilter *novelFilterIn `json:"novel_filter,omitempty"`
	pageLimitIn
}

// novelFollow 读取当前认证账号关注作者的小说新作；这是 App OAuth-only 流，
// 不用匿名 Web 搜索结果替代。
func (a *App) novelFollow(ctx context.Context, _ *mcp.CallToolRequest, in novelFollowIn) (*mcp.CallToolResult, novelListOut, error) {
	if in.Restrict == "" {
		in.Restrict = string(pixiv.RestrictPublic)
	}
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	ctx, err = withNovelFilter(ctx, in.NovelFilter)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.FollowingNovels(ctx, pixiv.FollowingNovelsRequest{Restrict: pixiv.Restrict(in.Restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return a.novelListError(ctx, err)
	}
	records, err := recordsFromNovels(items)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	out := novelListOut{Records: records, Pagination: listPagination(plan, in.Limit, len(items), more)}
	return novelListResult(out, false), out, nil
}

type latestIllustIn struct {
	ContentType  pixiv.SearchContentType `json:"content_type" jsonschema:"required: illust or manga"`
	Filter       string                  `json:"filter,omitempty" jsonschema:"safe local illustration filter expression"`
	IllustFilter *illustFilterIn         `json:"illust_filter,omitempty"`
	pageLimitIn
}

func (a *App) illustNew(ctx context.Context, _ *mcp.CallToolRequest, in latestIllustIn) (*mcp.CallToolResult, illustQueryOut, error) {
	if in.ContentType != pixiv.SearchContentTypeIllust && in.ContentType != pixiv.SearchContentTypeManga {
		return a.illustQueryError(ctx, errors.New("content_type must be one of: illust, manga"))
	}
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.illustQueryError(ctx, err)
	}
	ctx, err = withIllustFilterExpression(ctx, in.IllustFilter, in.Filter)
	if err != nil {
		return a.illustQueryError(ctx, err)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.illustQueryError(ctx, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.LatestArtworks(ctx, pixiv.LatestArtworksRequest{ContentType: in.ContentType, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return a.illustQueryError(ctx, err)
	}
	records, err := recordsFromArtworks(items)
	if err != nil {
		return a.illustQueryError(ctx, err)
	}
	out := illustQueryOut{Records: records, Pagination: listPagination(plan, in.Limit, len(items), more)}
	return illustQueryResult(out, false), out, nil
}

type latestNovelsIn struct {
	NovelFilter *novelFilterIn `json:"novel_filter,omitempty"`
	pageLimitIn
}

func (a *App) novelNew(ctx context.Context, _ *mcp.CallToolRequest, in latestNovelsIn) (*mcp.CallToolResult, novelListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	ctx, err = withNovelFilter(ctx, in.NovelFilter)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.LatestNovels(ctx, pixiv.LatestNovelsRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return a.novelListError(ctx, err)
	}
	records, err := recordsFromNovels(items)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	out := novelListOut{Records: records, Pagination: listPagination(plan, in.Limit, len(items), more)}
	return novelListResult(out, false), out, nil
}

type myPixivUsersIn struct {
	UserFilter *userFilterIn `json:"user_filter,omitempty"`
	pageLimitIn
}

// myPixivUsers 使用同一认证 snapshot 先解析当前 UID，再读取该账号的 MyPixiv
// 用户列表；不接受外部 UID 以免误表达为可浏览任意用户的私有关系。
func (a *App) myPixivUsers(ctx context.Context, _ *mcp.CallToolRequest, in myPixivUsersIn) (*mcp.CallToolResult, userListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.userListError(ctx, err)
	}
	ctx, err = withUserFilter(ctx, in.UserFilter)
	if err != nil {
		return a.userListError(ctx, err)
	}
	client, _, release, err := a.currentSDKUser(ctx)
	if err != nil {
		return a.userListError(ctx, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.MyPixivUsers(ctx, pixiv.MyPixivUsersRequest{Cursor: cursor})
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

type myPixivIllustsIn struct {
	Filter       string          `json:"filter,omitempty" jsonschema:"safe local illustration filter expression"`
	IllustFilter *illustFilterIn `json:"illust_filter,omitempty"`
	pageLimitIn
}

type myPixivNovelsIn struct {
	NovelFilter *novelFilterIn `json:"novel_filter,omitempty"`
	pageLimitIn
}

func (a *App) myPixivIllusts(ctx context.Context, _ *mcp.CallToolRequest, in myPixivIllustsIn) (*mcp.CallToolResult, illustQueryOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.illustQueryError(ctx, err)
	}
	ctx, err = withIllustFilterExpression(ctx, in.IllustFilter, in.Filter)
	if err != nil {
		return a.illustQueryError(ctx, err)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.illustQueryError(ctx, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.MyPixivArtworks(ctx, pixiv.MyPixivArtworksRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return a.illustQueryError(ctx, err)
	}
	records, err := recordsFromArtworks(items)
	if err != nil {
		return a.illustQueryError(ctx, err)
	}
	out := illustQueryOut{Records: records, Pagination: listPagination(plan, in.Limit, len(items), more)}
	return illustQueryResult(out, false), out, nil
}

func (a *App) myPixivNovels(ctx context.Context, _ *mcp.CallToolRequest, in myPixivNovelsIn) (*mcp.CallToolResult, novelListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	ctx, err = withNovelFilter(ctx, in.NovelFilter)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.MyPixivNovels(ctx, pixiv.MyPixivNovelsRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return a.novelListError(ctx, err)
	}
	records, err := recordsFromNovels(items)
	if err != nil {
		return a.novelListError(ctx, err)
	}
	out := novelListOut{Records: records, Pagination: listPagination(plan, in.Limit, len(items), more)}
	return novelListResult(out, false), out, nil
}

type userNovelsIn struct {
	UserID      int64          `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	NovelFilter *novelFilterIn `json:"novel_filter,omitempty"`
	pageLimitIn
}

type userNovelListOut struct {
	Records    []recordpkg.Record `json:"records"`
	Pagination paginationOut      `json:"pagination"`
}

func userNovelListResult(out userNovelListOut, isError bool) *mcp.CallToolResult {
	return recordResult(out.Records, isError, "")
}

func (a *App) userNovelListError(ctx context.Context, err error) (*mcp.CallToolResult, userNovelListOut, error) {
	out := userNovelListOut{Records: []recordpkg.Record{}, Pagination: paginationOut{Page: 1}}
	return recordResult(out.Records, true, recordErrorMessage(err)), out, nil
}

func (a *App) userNovels(ctx context.Context, _ *mcp.CallToolRequest, in userNovelsIn) (*mcp.CallToolResult, userNovelListOut, error) {
	plan, err := parseMCPListPlan(in.pageLimitIn)
	if err != nil {
		return a.userNovelListError(ctx, err)
	}
	ctx, err = withNovelFilter(ctx, in.NovelFilter)
	if err != nil {
		return a.userNovelListError(ctx, err)
	}
	client, userID, release, err := resolveSDKUser(ctx, a, in.UserID)
	if err != nil {
		return a.userNovelListError(ctx, err)
	}
	defer release()
	items, more, err := collectPages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.UserNovels(ctx, pixiv.UserNovelsRequest{UserID: userID, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return a.userNovelListError(ctx, err)
	}
	records, err := recordsFromNovels(items)
	if err != nil {
		return a.userNovelListError(ctx, err)
	}
	out := userNovelListOut{Records: records, Pagination: listPagination(plan, in.Limit, len(items), more)}
	return userNovelListResult(out, false), out, nil
}
