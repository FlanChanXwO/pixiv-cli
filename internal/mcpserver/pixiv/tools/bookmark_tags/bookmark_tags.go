// Package bookmark_tags 实现 bookmark_tags tool。
package bookmark_tags

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 bookmark_tags。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "bookmark_tags", Description: "List artwork bookmark tags.", OutputSchema: records.BookmarkTagsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.BookmarkTags, error) {
		return handleBookmarkTags(ctx, app, input)
	})
}

type In struct {
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	runtime.PageLimitIn
}

func handleBookmarkTags(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.BookmarkTags, error) {
	userID, err := runtime.ResolveUserID(app, ctx, in.UserID)
	if err != nil {
		return outputs.BookmarkTagsError(err)
	}
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.BookmarkTagsError(err)
	}
	items, more, err := runtime.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.BookmarkTag, sdk.Cursor, error) {
		result, err := client.UserArtworkBookmarkTags(ctx, pixiv.UserArtworkBookmarkTagsRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.BookmarkTagsError(err)
	}
	tags := make([]pixiv.BookmarkTagDTO, 0, len(items))
	for _, item := range items {
		tags = append(tags, pixiv.ToBookmarkTagDTO(item))
	}
	out := outputs.BookmarkTags{Tags: tags, Pagination: runtime.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.BookmarkTagsResult(out, len(items)), out, nil
}
