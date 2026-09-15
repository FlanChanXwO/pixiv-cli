// Package delete_artwork_comment 实现 delete_artwork_comment tool。
package delete_artwork_comment

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 delete_artwork_comment。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:        "delete_artwork_comment",
		Description: "Delete a Pixiv artwork comment by comment ID.",
		InputSchema: schemas.ClosedObject(map[string]any{
			"comment_id": schemas.PositiveInteger("Positive Pixiv comment ID."),
		}, []string{"comment_id"}),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleDeleteArtworkComment(ctx, app, input)
	})
}

// In 只接受要删除的可靠评论 ID；不会先读取最新评论再猜测目标。
type In struct {
	CommentID int64 `json:"comment_id" jsonschema:"positive comment ID"`
}

func handleDeleteArtworkComment(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{
		Action:    "delete_artwork_comment",
		CommentID: in.CommentID,
		Text:      fmt.Sprintf("Deleted comment %d.", in.CommentID),
	}
	return outputs.RunMutation(out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.DeleteArtworkComment(ctx, pixiv.DeleteArtworkCommentRequest{CommentID: in.CommentID})
		})
	})
}
