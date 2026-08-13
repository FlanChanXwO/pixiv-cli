// Package user_novel_bookmarks 实现 user_novel_bookmarks tool。
package user_novel_bookmarks

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 user_novel_bookmarks。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "user_novel_bookmarks", Description: "Browse user's bookmarked novels.", OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handleUserNovelBookmarks(ctx, app, input)
	})
}

type In struct {
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	Tag      string `json:"tag,omitempty"`
	runtime.PageLimitIn
}

func handleUserNovelBookmarks(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	userID, err := runtime.ResolveUserID(app, ctx, in.UserID)
	if err != nil {
		return outputs.Error(err)
	}
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := runtime.CollectPooledPages(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.UserNovelBookmarks(ctx, pixiv.UserNovelBookmarksRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Tag: in.Tag, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := records.FromNovels(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: runtime.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out, false), out, nil
}
