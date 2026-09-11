// Package novel_bookmark_detail 实现 novel_bookmark_detail tool。
package novel_bookmark_detail

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 novel_bookmark_detail。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "novel_bookmark_detail", Description: "Get the current user's bookmark state for one novel.", OutputSchema: records.BookmarkDetailOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.BookmarkDetail, error) {
		return handleNovelBookmarkDetail(ctx, app, input)
	})
}

type In struct {
	NovelID int64 `json:"novel_id" jsonschema:"positive novel ID"`
}

func handleNovelBookmarkDetail(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.BookmarkDetail, error) {
	if in.NovelID <= 0 {
		return outputs.BookmarkDetailError(errors.New("novel_id must be a positive integer"))
	}
	result, err := runtime.Read(app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.NovelBookmarkDetail, error) {
		return client.NovelBookmark(ctx, pixiv.NovelBookmarkRequest{NovelID: in.NovelID})
	})
	if err != nil {
		return outputs.BookmarkDetailError(err)
	}
	out := outputs.BookmarkDetail{Bookmarked: result.Restrict != "", Restrict: string(result.Restrict), Tags: append([]string(nil), result.Tags...)}
	return outputs.NovelBookmarkDetailResult(out, in.NovelID), out, nil
}
