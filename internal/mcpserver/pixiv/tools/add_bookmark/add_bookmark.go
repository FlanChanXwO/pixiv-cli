// Package add_bookmark 实现 add_bookmark tool。
package add_bookmark

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 add_bookmark。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "add_bookmark", Description: "Add an artwork to bookmarks."}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleAddBookmark(ctx, app, input)
	})
}

type In struct {
	IllustID int64    `json:"illust_id" jsonschema:"artwork ID"`
	Restrict string   `json:"restrict,omitempty" jsonschema:"public or private"`
	Tags     []string `json:"tags,omitempty" jsonschema:"bookmark tags"`
}

func handleAddBookmark(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{Action: "add_bookmark", IllustID: in.IllustID, Text: fmt.Sprintf("Bookmarked artwork %d.", in.IllustID)}
	return outputs.RunMutation(out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.AddBookmark(ctx, pixiv.AddBookmarkRequest{ArtworkID: in.IllustID, Restrict: pixiv.Restrict(in.Restrict), Tags: in.Tags})
		})
	})
}
