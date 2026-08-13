// Package home 实现 fanbox_home tool。
package home

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_home。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "fanbox_home", Description: "Browse the FANBOX home feed."}, func(ctx context.Context, request *mcp.CallToolRequest, input runtime.ListIn) (*mcp.CallToolResult, runtime.PostsOut, error) {
		return handle(ctx, app, input)
	})
}

func handle(ctx context.Context, app *runtime.App, input runtime.ListIn) (*mcp.CallToolResult, runtime.PostsOut, error) {
	out := runtime.PostsOut{Posts: []runtime.PostOut{}}
	plan, err := runtime.ParseListPlan(input)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	client, err := app.OpenClient(ctx)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	return runtime.PostList(ctx, app, client, input.Limit, plan, func(ctx context.Context, client *fanbox.Client, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		return client.Home(ctx, fanbox.HomeRequest{Cursor: cursor})
	})
}
