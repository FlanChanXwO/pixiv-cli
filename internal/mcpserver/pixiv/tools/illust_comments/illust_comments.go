// Package illust_comments 实现 illust_comments tool。
package illust_comments

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 illust_comments。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "illust_comments", Description: "Read comments for a Pixiv artwork.", OutputSchema: records.CommentOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Comments, error) {
		return handleIllustComments(ctx, app, input)
	})
}

type In struct {
	ID int64 `json:"id" jsonschema:"positive artwork or novel ID"`
	runtime.PageLimitIn
}

func handleIllustComments(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Comments, error) {
	return outputs.ListComments(ctx, app, in.ID, false, in.PageLimitIn, in.Limit)
}
