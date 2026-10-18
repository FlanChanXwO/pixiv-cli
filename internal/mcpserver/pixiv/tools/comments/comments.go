// Package comments 实现 pixiv_comments tool。
package comments

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_comments。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_comments", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Read comments for a Pixiv artwork or novel.", InputSchema: inputSchema(),
		OutputSchema: records.CommentOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Comments, error) {
		return handleIllustComments(ctx, app, input)
	})
}

// In 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type In struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id" jsonschema:"positive artwork or novel ID"`
	runtime.PageLimitIn
}

func handleIllustComments(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Comments, error) {
	return outputs.ListComments(ctx, app, in.ID, in.Kind == "novel", in.PageLimitIn, in.Limit)
}

func inputSchema() map[string]any {
	schema := records.CommentInputSchema()
	schema["properties"].(map[string]any)["kind"] = schemas.EnumString("Target content type.", "artwork", "novel")
	schema["required"] = []string{"kind", "id"}
	return schema
}
