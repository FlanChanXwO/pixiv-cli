// Package related_users 实现 related_users tool。
package related_users

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 related_users。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "related_users", Description: "Find users related to a Pixiv user.", InputSchema: schemas.List(map[string]any{
		"user_id":     schemas.PositiveInteger("Optional positive Pixiv user ID; defaults to the authenticated user."),
		"restrict":    schemas.EnumString("Compatibility visibility field.", "public", "private"),
		"user_filter": filters.UserFilterSchema(),
	}), OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handleRelatedUsers(ctx, app, input)
	})
}

type In struct {
	UserID     int64               `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict   string              `json:"restrict,omitempty" jsonschema:"public or private"`
	UserFilter *filters.UserFilter `json:"user_filter,omitempty"`
	runtime.PageLimitIn
}

func handleRelatedUsers(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	var err error
	ctx, err = filters.WithUserFilter(ctx, in.UserFilter)
	if err != nil {
		return outputs.Error(err)
	}
	return outputs.ListUserRelations(ctx, app, "related", in.UserID, in.Restrict, in.PageLimitIn, in.Limit)
}
