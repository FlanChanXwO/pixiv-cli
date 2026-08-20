// Package unfollow_user 实现 unfollow_user tool。
package unfollow_user

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 unfollow_user。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "unfollow_user", Description: "Unfollow a Pixiv user."}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleUnfollowUser(ctx, app, input)
	})
}

type In struct {
	UserID int64 `json:"user_id" jsonschema:"user ID"`
}

func handleUnfollowUser(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{Action: "unfollow_user", UserID: in.UserID, Text: fmt.Sprintf("Unfollowed user %d.", in.UserID)}
	return outputs.RunMutation(out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.UnfollowUser(ctx, pixiv.UnfollowUserRequest{UserID: in.UserID})
		})
	})
}
