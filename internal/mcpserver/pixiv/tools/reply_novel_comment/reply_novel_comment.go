// Package reply_novel_comment 实现 reply_novel_comment tool。
package reply_novel_comment

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 reply_novel_comment。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:        "reply_novel_comment",
		Description: "Reply to a comment on a Pixiv novel.",
		InputSchema: schemas.ClosedObject(novelCommentProperties(), []string{"novel_id", "comment", "parent_comment_id"}),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleReplyNovelComment(ctx, app, input)
	})
}

// In 的 parent_comment_id 是明确的上游父评论 ID，不通过评论列表推测。
type In struct {
	NovelID         int64  `json:"novel_id" jsonschema:"positive novel ID"`
	Comment         string `json:"comment" jsonschema:"comment body"`
	ParentCommentID int64  `json:"parent_comment_id" jsonschema:"positive parent comment ID"`
}

func novelCommentProperties() map[string]any {
	return map[string]any{
		"novel_id":          schemas.PositiveInteger("Positive Pixiv novel ID."),
		"comment":           map[string]any{"type": "string", "minLength": 1, "description": "Non-empty comment body."},
		"parent_comment_id": schemas.PositiveInteger("Positive parent comment ID."),
	}
}

func handleReplyNovelComment(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{
		Action:  "reply_novel_comment",
		NovelID: in.NovelID,
		Text:    fmt.Sprintf("Replied to comment %d on novel %d.", in.ParentCommentID, in.NovelID),
	}
	return outputs.RunMutationInPlace(&out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			result, err := client.ReplyNovelComment(ctx, pixiv.ReplyNovelCommentRequest{
				NovelID:         in.NovelID,
				Comment:         in.Comment,
				ParentCommentID: in.ParentCommentID,
			})
			if err != nil {
				return err
			}
			out.CommentID = result.CommentID
			return nil
		})
	})
}
