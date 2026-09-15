// Package remove_novel_bookmark 实现 remove_novel_bookmark tool。
package remove_novel_bookmark

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 remove_novel_bookmark。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "remove_novel_bookmark", Description: "Remove a novel from bookmarks."}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleRemoveNovelBookmark(ctx, app, input)
	})
}

type In struct {
	NovelID int64 `json:"novel_id" jsonschema:"novel ID"`
}

func handleRemoveNovelBookmark(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{Action: "remove_novel_bookmark", NovelID: in.NovelID, Text: fmt.Sprintf("Removed bookmark from novel %d.", in.NovelID)}
	return outputs.RunMutation(out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.RemoveNovelBookmark(ctx, pixiv.RemoveNovelBookmarkRequest{NovelID: in.NovelID})
		})
	})
}
