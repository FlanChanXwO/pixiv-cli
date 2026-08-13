// Package novel_content 实现 novel_content tool。
package novel_content

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 novel_content。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "novel_content", Description: "Read the complete structured content of one Pixiv novel.", OutputSchema: records.NovelContentOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.NovelContent, error) {
		return handleNovelContent(ctx, app, input)
	})
}

type In struct {
	NovelID int64 `json:"novel_id" jsonschema:"positive Pixiv novel ID"`
}

func handleNovelContent(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.NovelContent, error) {
	if in.NovelID <= 0 {
		return outputs.NovelContentError(errors.New("novel_id must be a positive integer"))
	}
	result, err := runtime.Read(app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.NovelContent, error) {
		return client.NovelContent(ctx, pixiv.NovelContentRequest{NovelID: in.NovelID})
	})
	if err != nil {
		return outputs.NovelContentError(err)
	}
	out := outputs.NovelContent{Content: pixiv.ToNovelContentDTO(result)}
	return outputs.NovelContentResult(result.NovelID), out, nil
}
