// Package user_relations 实现 pixiv_user_relations tool。
package user_relations

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_user_relations。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_user_relations", Description: "Browse followers, following or related users; restrict applies only to followers/following.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema:  schemas.List(map[string]any{"relation": schemas.EnumString("User relation.", "followers", "following", "related"), "user_id": schemas.PositiveInteger("Optional user ID; defaults to authenticated user."), "restrict": schemas.EnumString("Followers/following visibility; ignored for related users.", "public", "private"), "user_filter": filters.UserFilterSchema()}, "relation"),
		OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handle(ctx, app, input)
	})
}

// In 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type In struct {
	Relation   string              `json:"relation"`
	UserID     int64               `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict   string              `json:"restrict,omitempty" jsonschema:"public or private"`
	UserFilter *filters.UserFilter `json:"user_filter,omitempty"`
	runtime.PageLimitIn
}

func handle(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	if in.Relation == "followers" && in.UserFilter != nil {
		return outputs.Error(errors.New("user_filter is only supported for following and related users"))
	}
	if _, err := runtime.ParseListPlan(in.PageLimitIn); err != nil {
		return outputs.Error(err)
	}
	ctx, err := filters.WithUserFilter(ctx, in.UserFilter)
	if err != nil {
		return outputs.Error(err)
	}
	return outputs.ListUserRelations(ctx, app, in.Relation, in.UserID, in.Restrict, in.PageLimitIn, in.Limit)
}
