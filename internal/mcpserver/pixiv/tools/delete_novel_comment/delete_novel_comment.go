// Package delete_novel_comment 实现 delete_novel_comment tool。
package delete_novel_comment

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 delete_novel_comment。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:        "delete_novel_comment",
		Description: "Delete a Pixiv novel comment by comment ID.",
		InputSchema: schemas.ClosedObject(map[string]any{
			"comment_id": schemas.PositiveInteger("Positive Pixiv comment ID."),
		}, []string{"comment_id"}),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleDeleteNovelComment(ctx, app, input)
	})
}

// In 只接受要删除的可靠评论 ID；不会先读取最新评论再猜测目标。
type In struct {
	CommentID int64 `json:"comment_id" jsonschema:"positive comment ID"`
}

func handleDeleteNovelComment(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{
		Action:    "delete_novel_comment",
		CommentID: in.CommentID,
		Text:      fmt.Sprintf("Deleted comment %d.", in.CommentID),
	}
	return outputs.RunMutation(out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.DeleteNovelComment(ctx, pixiv.DeleteNovelCommentRequest{CommentID: in.CommentID})
		})
	})
}
