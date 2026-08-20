// Package novel_detail 实现 novel_detail tool。
package novel_detail

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 novel_detail。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "novel_detail", Description: "Get detailed information for one Pixiv novel.", OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.NovelDetail, error) {
		return handleNovelDetail(ctx, app, input)
	})
}

type In struct {
	NovelID int64 `json:"novel_id" jsonschema:"positive Pixiv novel ID"`
}

func handleNovelDetail(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.NovelDetail, error) {
	if in.NovelID <= 0 {
		return outputs.NovelDetailError(errors.New("novel_id must be a positive integer"))
	}
	result, err := runtime.Read(app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.Novel, error) {
		return client.Novel(ctx, pixiv.NovelRequest{NovelID: in.NovelID})
	})
	if err != nil {
		return outputs.NovelDetailError(err)
	}
	novelRecord, err := records.FromNovel(result)
	if err != nil {
		return outputs.NovelDetailError(err)
	}
	out := outputs.NovelDetail{Records: []record.Record{novelRecord}}
	return outputs.NovelDetailResult(out), out, nil
}
