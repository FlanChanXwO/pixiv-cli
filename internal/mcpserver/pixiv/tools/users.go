// pixiv users tools.

package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerUserArtworks 注册 pixiv_user_artworks。
func (t pixivTools) registerUserArtworks(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_user_artworks", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Browse a user's artworks.", InputSchema: schemas.List(map[string]any{
		"user_id":       schemas.PositiveInteger("Optional positive Pixiv user ID; defaults to the authenticated user."),
		"type":          schemas.EnumString("Artwork type.", "illust", "manga", "ugoira"),
		"illust_filter": filters.IllustFilterSchema(),
	}), OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input userArtworksIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.userArtworksHandler(ctx, input)
	})
}

type userArtworksIn struct {
	UserID       int64                 `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	Type         string                `json:"type,omitempty" jsonschema:"illust, manga, or ugoira"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	toolrun.ListIn
}

func (t pixivTools) userArtworksHandler(ctx context.Context, in userArtworksIn) (*mcp.CallToolResult, outputs.Records, error) {
	plan, err := toolrun.ParseListPlan(in.ListIn)
	if err != nil {
		return outputs.Error(err)
	}
	ctx, err = filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	userID, err := toolhost.ResolveUserID(t.app, ctx, in.UserID)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: userID, Kind: pixiv.ArtworkKind(in.Type), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := record.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out), out, nil
}

// registerUserBookmarks 注册 pixiv_user_bookmarks。
func (t pixivTools) registerUserBookmarks(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_user_bookmarks", InputSchema: userBookmarksInputSchema(), Description: "Browse a user's bookmarked artworks or novels.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input userBookmarksIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.userBookmarksHandler(ctx, input)
	})
}

// userBookmarksIn 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type userBookmarksIn struct {
	Kind         string                `json:"kind" jsonschema:"content type"`
	UserID       int64                 `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	Restrict     string                `json:"restrict,omitempty" jsonschema:"public or private"`
	Tag          string                `json:"tag,omitempty"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	toolrun.ListIn
}

func (t pixivTools) userBookmarksHandler(ctx context.Context, in userBookmarksIn) (*mcp.CallToolResult, outputs.Records, error) {
	plan, err := toolrun.ParseListPlan(in.ListIn)
	if err != nil {
		return outputs.Error(err)
	}
	if in.Kind == "novel" && in.IllustFilter != nil {
		return outputs.Error(errors.New("illust_filter requires kind=artwork"))
	}
	if in.Kind == "novel" {
		userID, err := toolhost.ResolveUserID(t.app, ctx, in.UserID)
		if err != nil {
			return outputs.Error(err)
		}
		items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.UserNovelBookmarks(ctx, pixiv.UserNovelBookmarksRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Tag: in.Tag, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		})
		if err != nil {
			return outputs.Error(err)
		}
		recordItems, err := record.FromNovels(items)
		if err != nil {
			return outputs.Error(err)
		}
		out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
		return outputs.Result(out), out, nil
	}

	userID := in.UserID
	ctx, err = filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	userID, err = toolhost.ResolveUserID(t.app, ctx, userID)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.UserArtworkBookmarks(ctx, pixiv.UserArtworkBookmarksRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Tag: in.Tag, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := record.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out), out, nil
}

func userBookmarksInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[userBookmarksIn](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["kind"].Enum = []any{"artwork", "novel"}
	return schema
}

// registerUserDetail 注册 pixiv_user_detail。
func (t pixivTools) registerUserDetail(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_user_detail", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get a user's complete profile through the authenticated Pixiv SDK.", InputSchema: schemas.ClosedObject(map[string]any{
		"user_id": schemas.PositiveInteger("Positive Pixiv user ID."),
	}, []string{"user_id"}), OutputSchema: schemas.DetailOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input userDetailIn) (*mcp.CallToolResult, outputs.Detail, error) {
		return t.userDetailHandler(ctx, input)
	})
}

// userDetailIn 只接受明确指定的目标用户，避免把请求误解析为当前认证账号。
type userDetailIn struct {
	UserID int64 `json:"user_id" jsonschema:"required positive Pixiv user ID"`
}

// userDetailHandler 将完整公开 SDK envelope 封装为单条 user record，避免详情 tool
// 成为 records 契约的例外。
func (t pixivTools) userDetailHandler(ctx context.Context, in userDetailIn) (*mcp.CallToolResult, outputs.Detail, error) {
	if in.UserID <= 0 {
		return outputs.DetailError(fmt.Errorf("user_id must be a positive integer"))
	}
	result, err := toolhost.Read(t.app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.UserDetail, error) {
		return client.User(ctx, pixiv.UserRequest{UserID: in.UserID})
	})
	if err != nil {
		return outputs.DetailError(err)
	}
	userRecord, err := record.RecordFromUserDetail(result)
	if err != nil {
		return outputs.DetailError(err)
	}
	out := outputs.Detail{Records: []record.Record{userRecord}}
	return outputs.DetailResult(out), out, nil
}

// registerUserNovels 注册 pixiv_user_novels。
func (t pixivTools) registerUserNovels(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_user_novels", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Browse a user's novels through the App API.", InputSchema: schemas.List(map[string]any{
		"user_id":      schemas.PositiveInteger("Optional positive Pixiv user ID; defaults to the authenticated user."),
		"novel_filter": filters.NovelFilterSchema(),
	}), OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input userNovelsIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.userNovelsHandler(ctx, input)
	})
}

type userNovelsIn struct {
	UserID      int64                `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to the authenticated user"`
	NovelFilter *filters.NovelFilter `json:"novel_filter,omitempty"`
	toolrun.ListIn
}

func (t pixivTools) userNovelsHandler(ctx context.Context, in userNovelsIn) (*mcp.CallToolResult, outputs.Records, error) {
	plan, err := toolrun.ParseListPlan(in.ListIn)
	if err != nil {
		return outputs.Error(err)
	}
	ctx, err = filters.WithNovelFilter(ctx, in.NovelFilter)
	if err != nil {
		return outputs.Error(err)
	}
	userID, err := toolhost.ResolveUserID(t.app, ctx, in.UserID)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.UserNovels(ctx, pixiv.UserNovelsRequest{UserID: userID, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := record.FromNovels(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out), out, nil
}
