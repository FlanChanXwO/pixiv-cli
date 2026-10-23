// Package comment_delete 实现 pixiv_comment_delete tool。
package comment_delete

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_comment_delete。
func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "pixiv_comment_delete", Description: "Delete an artwork or novel comment by comment ID.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(map[string]any{"kind": schemas.EnumString("Target content type.", "artwork", "novel"), "comment_id": schemas.PositiveInteger("Positive Pixiv comment ID.")}, []string{"kind", "comment_id"})}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handle(ctx, app, input)
	})
}

// In 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type In struct {
	Kind      string `json:"kind"`
	CommentID int64  `json:"comment_id" jsonschema:"positive comment ID"`
}

func handle(ctx context.Context, app *toolhost.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == "novel" {
		out := outputs.Mutation{
			Action:    "delete_novel_comment",
			CommentID: in.CommentID,
			Text:      fmt.Sprintf("Deleted comment %d.", in.CommentID),
		}
		return outputs.RunMutation(out, func() error {
			return toolhost.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
				return client.DeleteNovelComment(ctx, pixiv.DeleteNovelCommentRequest{CommentID: in.CommentID})
			})
		})
	}

	out := outputs.Mutation{
		Action:    "delete_artwork_comment",
		CommentID: in.CommentID,
		Text:      fmt.Sprintf("Deleted comment %d.", in.CommentID),
	}
	return outputs.RunMutation(out, func() error {
		return toolhost.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.DeleteArtworkComment(ctx, pixiv.DeleteArtworkCommentRequest{CommentID: in.CommentID})
		})
	})
}
