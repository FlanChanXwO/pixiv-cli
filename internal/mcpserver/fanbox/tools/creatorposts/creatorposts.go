// Package creatorposts 实现 fanbox_creator_posts tool。
package creatorposts

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_creator_posts。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "fanbox_creator_posts", Description: "List posts from a FANBOX creator."}, func(ctx context.Context, request *mcp.CallToolRequest, input in) (*mcp.CallToolResult, runtime.PostsOut, error) {
		return handle(ctx, app, input)
	})
}

type in struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
	runtime.ListIn
}

func handle(ctx context.Context, app *runtime.App, input in) (*mcp.CallToolResult, runtime.PostsOut, error) {
	out := runtime.PostsOut{Posts: []runtime.PostOut{}}
	plan, err := runtime.ParseListPlan(input.ListIn)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	if input.CreatorID == "" {
		return runtime.Result(out, true, "Error: creator_id is required"), out, nil
	}
	lease, err := app.OpenClient(ctx)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer lease.Close()
	client := lease.Value()
	creatorID := input.CreatorID
	return runtime.PostList(ctx, app, client, input.Limit, plan, func(ctx context.Context, client *fanbox.Client, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		return client.CreatorPosts(ctx, fanbox.CreatorPostsRequest{CreatorID: creatorID, Cursor: cursor})
	})
}
