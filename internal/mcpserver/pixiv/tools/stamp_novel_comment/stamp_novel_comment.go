// Package stamp_novel_comment 实现 stamp_novel_comment tool。
package stamp_novel_comment

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 stamp_novel_comment。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:        "stamp_novel_comment",
		Description: "Add a stamp comment to a Pixiv novel.",
		InputSchema: schemas.ClosedObject(novelCommentProperties(), []string{"novel_id", "comment", "stamp_id"}),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleStampNovelComment(ctx, app, input)
	})
}

// In 把 stamp_id 作为独立的上游字段传递，不把它当作回复父评论 ID。
type In struct {
	NovelID int64  `json:"novel_id" jsonschema:"positive novel ID"`
	Comment string `json:"comment" jsonschema:"comment body"`
	StampID int64  `json:"stamp_id" jsonschema:"positive stamp ID"`
}

func novelCommentProperties() map[string]any {
	return map[string]any{
		"novel_id": schemas.PositiveInteger("Positive Pixiv novel ID."),
		"comment":  map[string]any{"type": "string", "minLength": 1, "description": "Non-empty comment body."},
		"stamp_id": schemas.PositiveInteger("Positive Pixiv stamp ID."),
	}
}

func handleStampNovelComment(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{
		Action:  "stamp_novel_comment",
		NovelID: in.NovelID,
		Text:    fmt.Sprintf("Added stamp comment to novel %d.", in.NovelID),
	}
	return outputs.RunMutationInPlace(&out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			result, err := client.StampNovelComment(ctx, pixiv.StampNovelCommentRequest{
				NovelID: in.NovelID,
				Comment: in.Comment,
				StampID: in.StampID,
			})
			if err != nil {
				return err
			}
			out.CommentID = result.CommentID
			return nil
		})
	})
}
