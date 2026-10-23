// Package blocked_users 实现 pixiv_blocked_users tool。
package blocked_users

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_blocked_users。
func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "pixiv_blocked_users", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "View users blocked by the authenticated account through the App API.", InputSchema: schemas.List(map[string]any{
		"user_id":  schemas.PositiveInteger("Optional positive Pixiv user ID; defaults to the authenticated user."),
		"restrict": schemas.EnumString("Compatibility visibility field.", "public", "private"),
	}), OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handleBlockedUsers(ctx, app, input)
	})
}

type In struct {
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	toolhost.PageLimitIn
}

func handleBlockedUsers(ctx context.Context, app *toolhost.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	return outputs.ListUserRelations(ctx, app, "blocked", in.UserID, in.Restrict, in.PageLimitIn, in.Limit)
}
