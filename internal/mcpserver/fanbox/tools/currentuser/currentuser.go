// Package currentuser 实现 fanbox_current_user tool。
package currentuser

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_current_user。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "fanbox_current_user", Description: "Show the current authenticated FANBOX user."}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		return handle(ctx, app, input)
	})
}

type In struct{}

type Out struct {
	UserID        int64  `json:"user_id"`
	DisplayName   string `json:"display_name"`
	CreatorID     string `json:"creator_id"`
	CreatorStatus string `json:"creator_status"`
	IsCreator     bool   `json:"is_creator"`
}

func handle(ctx context.Context, app *runtime.App, _ In) (*mcp.CallToolResult, Out, error) {
	empty := Out{}
	lease, err := app.OpenClient(ctx)
	if err != nil {
		return runtime.Result(empty, true, "Error: "+err.Error()), empty, nil
	}
	defer lease.Close()
	client := lease.Value()
	user, err := client.CurrentUser(ctx, fanbox.CurrentUserRequest{})
	if err != nil {
		return runtime.Result(empty, true, "Error: "+err.Error()), empty, nil
	}
	out := Out{
		UserID:        user.UserID,
		DisplayName:   user.DisplayName,
		CreatorID:     user.CreatorID,
		CreatorStatus: user.CreatorStatus,
		IsCreator:     user.IsCreator,
	}
	return runtime.Result(out, false, fmt.Sprintf("Current FANBOX user %d.", out.UserID)), out, nil
}
