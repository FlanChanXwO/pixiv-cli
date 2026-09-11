// Package create_novel_comment 实现 create_novel_comment tool。
package create_novel_comment

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 create_novel_comment。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:        "create_novel_comment",
		Description: "Create a top-level comment on a Pixiv novel.",
		InputSchema: schemas.ClosedObject(novelCommentProperties(), []string{"novel_id", "comment"}),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleCreateNovelComment(ctx, app, input)
	})
}

// In 只接受明确的小说 ID 与评论正文；创建 ID 必须来自上游响应。
type In struct {
	NovelID int64  `json:"novel_id" jsonschema:"positive novel ID"`
	Comment string `json:"comment" jsonschema:"comment body"`
}

func novelCommentProperties() map[string]any {
	return map[string]any{
		"novel_id": schemas.PositiveInteger("Positive Pixiv novel ID."),
		"comment":  map[string]any{"type": "string", "minLength": 1, "description": "Non-empty comment body."},
	}
}

func handleCreateNovelComment(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{
		Action:  "create_novel_comment",
		NovelID: in.NovelID,
		Text:    fmt.Sprintf("Created comment on novel %d.", in.NovelID),
	}
	return outputs.RunMutationInPlace(&out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			result, err := client.PostNovelComment(ctx, pixiv.PostNovelCommentRequest{
				NovelID: in.NovelID,
				Comment: in.Comment,
			})
			if err != nil {
				return err
			}
			out.CommentID = result.CommentID
			return nil
		})
	})
}
