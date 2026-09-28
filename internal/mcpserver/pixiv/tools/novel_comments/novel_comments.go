// Package novel_comments 实现 pixiv_novel_comments tool。
package novel_comments

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_novel_comments。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_novel_comments", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Read comments for a Pixiv novel.", InputSchema: records.CommentInputSchema(), OutputSchema: records.CommentOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Comments, error) {
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
