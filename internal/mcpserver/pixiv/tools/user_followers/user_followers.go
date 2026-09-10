// Package user_followers 实现 user_followers tool。
package user_followers

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 user_followers。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "user_followers", Description: "View user's followers list.", InputSchema: schemas.List(map[string]any{
		"user_id":  schemas.PositiveInteger("Optional positive Pixiv user ID; defaults to the authenticated user."),
		"restrict": schemas.EnumString("Follower visibility.", "public", "private"),
	}), OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handleUserFollowers(ctx, app, input)
	})
}

type In struct {
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	runtime.PageLimitIn
}

func handleUserFollowers(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	return outputs.ListUserRelations(ctx, app, "followers", in.UserID, in.Restrict, in.PageLimitIn, in.Limit)
}
