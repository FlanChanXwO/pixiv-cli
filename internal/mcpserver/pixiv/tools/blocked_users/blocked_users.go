// Package blocked_users 实现 blocked_users tool。
package blocked_users

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 blocked_users。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "blocked_users", Description: "View users blocked by the authenticated account through the App API.", OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handleBlockedUsers(ctx, app, input)
	})
}

type In struct {
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	runtime.PageLimitIn
}

func handleBlockedUsers(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	return outputs.ListUserRelations(ctx, app, "blocked", in.UserID, in.Restrict, in.PageLimitIn, in.Limit)
}
