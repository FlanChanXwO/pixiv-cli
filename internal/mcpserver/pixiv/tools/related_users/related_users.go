// Package related_users 实现 related_users tool。
package related_users

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 related_users。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "related_users", Description: "Find users related to a Pixiv user.", OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handleRelatedUsers(ctx, app, input)
	})
}

type In struct {
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	runtime.PageLimitIn
}

func handleRelatedUsers(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	if in.UserID <= 0 {
		return outputs.Error(errors.New("user_id must be a positive integer"))
	}
	return outputs.ListUserRelations(ctx, app, "related", in.UserID, in.Restrict, in.PageLimitIn, in.Limit)
}
