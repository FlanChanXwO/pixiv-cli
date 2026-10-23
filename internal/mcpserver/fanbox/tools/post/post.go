// Package post 实现 fanbox_post tool。
package post

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/toolhost"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_post。
func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "fanbox_post", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get one FANBOX post."}, func(ctx context.Context, request *mcp.CallToolRequest, input in) (*mcp.CallToolResult, toolhost.PostOut, error) {
		return handle(ctx, app, input)
	})
}

type in struct {
	PostID string `json:"post_id" jsonschema:"required FANBOX post id"`
}

func handle(ctx context.Context, app *toolhost.App, input in) (*mcp.CallToolResult, toolhost.PostOut, error) {
	out := toolhost.PostOut{Assets: []toolhost.PostAssetOut{}}
	if input.PostID == "" {
		return toolhost.Result(out, true, "Error: post_id is required"), out, nil
	}
	lease, err := app.OpenClient(ctx)
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer lease.Close()
	client := lease.Value()
	post, err := client.Post(ctx, fanbox.PostRequest{PostID: input.PostID})
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	out = toolhost.PostOutFrom(post)
	return toolhost.Result(out, false, fmt.Sprintf("Retrieved post %s.", out.ID)), out, nil
}
