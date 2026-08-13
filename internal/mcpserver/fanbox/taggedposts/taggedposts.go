// Package taggedposts 实现 fanbox_tagged_posts tool。
package taggedposts

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_tagged_posts。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "fanbox_tagged_posts", Description: "List posts from a creator for one tag."}, func(ctx context.Context, request *mcp.CallToolRequest, input in) (*mcp.CallToolResult, runtime.PostsOut, error) {
		return handle(ctx, app, input)
	})
}

type in struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
	Tag       string `json:"tag" jsonschema:"required tag name"`
	runtime.ListIn
}

func handle(ctx context.Context, app *runtime.App, input in) (*mcp.CallToolResult, runtime.PostsOut, error) {
	out := runtime.PostsOut{Posts: []runtime.PostOut{}}
	plan, err := runtime.ParseListPlan(input.ListIn)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	if input.CreatorID == "" || input.Tag == "" {
		return runtime.Result(out, true, "Error: creator_id and tag are required"), out, nil
	}
	client, err := app.OpenClient(ctx)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	creatorID, tag := input.CreatorID, input.Tag
	return runtime.PostList(ctx, app, client, input.Limit, plan, func(ctx context.Context, client *fanbox.Client, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		return client.TaggedPosts(ctx, fanbox.TaggedPostsRequest{CreatorID: creatorID, Tag: tag, Cursor: cursor})
	})
}
