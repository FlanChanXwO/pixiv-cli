// Package follow_user 实现 follow_user tool。
package follow_user

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 follow_user。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "follow_user", Description: "Follow a Pixiv user."}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleFollowUser(ctx, app, input)
	})
}

type In struct {
	UserID   int64  `json:"user_id" jsonschema:"user ID"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
}

func handleFollowUser(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{Action: "follow_user", UserID: in.UserID, Text: fmt.Sprintf("Followed user %d.", in.UserID)}
	return outputs.RunMutation(out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.FollowUser(ctx, pixiv.FollowUserRequest{UserID: in.UserID, Restrict: pixiv.Restrict(in.Restrict)})
		})
	})
}
