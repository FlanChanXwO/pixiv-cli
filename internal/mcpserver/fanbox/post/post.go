// Package post 实现 fanbox_post tool。
package post

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_post。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "fanbox_post", Description: "Get one FANBOX post."}, func(ctx context.Context, request *mcp.CallToolRequest, input in) (*mcp.CallToolResult, runtime.PostOut, error) {
		return handle(ctx, app, input)
	})
}

type in struct {
	PostID string `json:"post_id" jsonschema:"required FANBOX post id"`
}

func handle(ctx context.Context, app *runtime.App, input in) (*mcp.CallToolResult, runtime.PostOut, error) {
	out := runtime.PostOut{Assets: []runtime.PostAssetOut{}}
	if input.PostID == "" {
		return runtime.Result(out, true, "Error: post_id is required"), out, nil
	}
	client, err := app.OpenClient(ctx)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	post, err := client.Post(ctx, fanbox.PostRequest{PostID: input.PostID})
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	out = runtime.PostOutFrom(post)
	return runtime.Result(out, false, fmt.Sprintf("Retrieved post %s.", out.ID)), out, nil
}
