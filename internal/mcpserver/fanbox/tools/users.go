// fanbox users tools.

package tools

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerCurrentuser 注册 fanbox_current_user。
func (t fanboxTools) registerCurrentuser(server *mcp.Server) {
	toolrun.AddTool(t.calls, server, &mcp.Tool{Name: "fanbox_current_user", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Show the current authenticated FANBOX user."}, func(ctx context.Context, request *mcp.CallToolRequest, input currentuserIn) (*mcp.CallToolResult, currentuserOut, error) {
		return t.currentuserHandler(ctx, input)
	})
}

type currentuserIn struct{}

type currentuserOut struct {
	UserID        int64  `json:"user_id"`
	DisplayName   string `json:"display_name"`
	CreatorID     string `json:"creator_id"`
	CreatorStatus string `json:"creator_status"`
	IsCreator     bool   `json:"is_creator"`
}

func (t fanboxTools) currentuserHandler(ctx context.Context, _ currentuserIn) (*mcp.CallToolResult, currentuserOut, error) {
	empty := currentuserOut{}
	lease, err := t.openClient(ctx)
	if err != nil {
		return outputs.Result(true, "Error: "+err.Error()), empty, nil
	}
	defer lease.Close()
	client := lease.Value()
	user, err := client.CurrentUser(ctx, fanbox.CurrentUserRequest{})
	if err != nil {
		return outputs.Result(true, "Error: "+err.Error()), empty, nil
	}
	out := currentuserOut{
		UserID:        user.UserID,
		DisplayName:   user.DisplayName,
		CreatorID:     user.CreatorID,
		CreatorStatus: user.CreatorStatus,
		IsCreator:     user.IsCreator,
	}
	return outputs.Result(false, fmt.Sprintf("Current FANBOX user %d.", out.UserID)), out, nil
}
