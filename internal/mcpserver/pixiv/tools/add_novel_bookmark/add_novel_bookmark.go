// Package add_novel_bookmark 实现 add_novel_bookmark tool。
package add_novel_bookmark

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 add_novel_bookmark。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "add_novel_bookmark", Description: "Add a novel to bookmarks."}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleAddNovelBookmark(ctx, app, input)
	})
}

type In struct {
	NovelID  int64    `json:"novel_id" jsonschema:"novel ID"`
	Restrict string   `json:"restrict,omitempty" jsonschema:"public or private"`
	Tags     []string `json:"tags,omitempty" jsonschema:"bookmark tags"`
}

func handleAddNovelBookmark(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{Action: "add_novel_bookmark", NovelID: in.NovelID, Text: fmt.Sprintf("Bookmarked novel %d.", in.NovelID)}
	return outputs.RunMutation(out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.AddNovelBookmark(ctx, pixiv.AddNovelBookmarkRequest{NovelID: in.NovelID, Restrict: pixiv.Restrict(in.Restrict), Tags: in.Tags})
		})
	})
}
