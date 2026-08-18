// Package novel_comments 实现 novel_comments tool。
package novel_comments

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 novel_comments。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "novel_comments", Description: "Read comments for a Pixiv novel.", OutputSchema: records.CommentOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Comments, error) {
		return handleNovelComments(ctx, app, input)
	})
}

type In struct {
	ID int64 `json:"id" jsonschema:"positive artwork or novel ID"`
	runtime.PageLimitIn
}

func handleNovelComments(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Comments, error) {
	return outputs.ListComments(ctx, app, in.ID, true, in.PageLimitIn, in.Limit)
}
