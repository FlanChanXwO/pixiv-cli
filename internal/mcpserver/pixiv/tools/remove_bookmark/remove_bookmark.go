// Package remove_bookmark 实现 remove_bookmark tool。
package remove_bookmark

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 remove_bookmark。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "remove_bookmark", Description: "Remove an artwork from bookmarks."}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleRemoveBookmark(ctx, app, input)
	})
}

type In struct {
	IllustID int64 `json:"illust_id" jsonschema:"artwork ID"`
}

func handleRemoveBookmark(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{Action: "remove_bookmark", IllustID: in.IllustID, Text: fmt.Sprintf("Removed bookmark from artwork %d.", in.IllustID)}
	return outputs.RunMutation(out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.RemoveBookmark(ctx, pixiv.RemoveBookmarkRequest{ArtworkID: in.IllustID})
		})
	})
}
