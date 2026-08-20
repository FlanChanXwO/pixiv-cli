// Package bookmark_detail 实现 bookmark_detail tool。
package bookmark_detail

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 bookmark_detail。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "bookmark_detail", Description: "Get the current user's bookmark state for one artwork.", OutputSchema: records.BookmarkDetailOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.BookmarkDetail, error) {
		return handleBookmarkDetail(ctx, app, input)
	})
}

type In struct {
	IllustID int64 `json:"illust_id" jsonschema:"positive artwork ID"`
}

func handleBookmarkDetail(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.BookmarkDetail, error) {
	if in.IllustID <= 0 {
		return outputs.BookmarkDetailError(errors.New("illust_id must be a positive integer"))
	}
	result, err := runtime.Read(app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.ArtworkBookmarkDetail, error) {
		return client.ArtworkBookmark(ctx, pixiv.ArtworkBookmarkRequest{ArtworkID: in.IllustID})
	})
	if err != nil {
		return outputs.BookmarkDetailError(err)
	}
	out := outputs.BookmarkDetail{Bookmarked: result.Restrict != "", Restrict: string(result.Restrict), Tags: append([]string(nil), result.Tags...)}
	return outputs.BookmarkDetailResult(out, in.IllustID), out, nil
}
