// Package comment_stamp 实现 pixiv_comment_stamp tool。
package comment_stamp

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_comment_stamp。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_comment_stamp", Description: "Add a stamp comment to an artwork or novel.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: false, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(commentProperties(), []string{"kind", "id", "stamp_id"})}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handle(ctx, app, input)
	})
}

// In 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type In struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id" jsonschema:"positive artwork or novel ID"`
	Comment string `json:"comment" jsonschema:"comment body"`
	StampID int64  `json:"stamp_id" jsonschema:"positive stamp ID"`
}

func commentProperties() map[string]any {
	return map[string]any{
		"kind":     schemas.EnumString("Target content type.", "artwork", "novel"),
		"id":       schemas.PositiveInteger("Positive Pixiv artwork or novel ID."),
		"comment":  map[string]any{"type": "string", "description": "Optional comment body; empty is the sticker-only wire form."},
		"stamp_id": schemas.PositiveInteger("Positive Pixiv stamp ID."),
	}
}

func handle(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == "novel" {
		out := outputs.Mutation{
			Action:  "stamp_novel_comment",
			NovelID: in.ID,
			Text:    fmt.Sprintf("Added stamp comment to novel %d.", in.ID),
		}
		return outputs.RunMutationInPlace(&out, func() error {
			return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
				result, err := client.StampNovelComment(ctx, pixiv.StampNovelCommentRequest{
					NovelID: in.ID,
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

	out := outputs.Mutation{
		Action:   "stamp_artwork_comment",
		IllustID: in.ID,
		Text:     fmt.Sprintf("Added stamp comment to artwork %d.", in.ID),
	}
	return outputs.RunMutationInPlace(&out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			result, err := client.StampArtworkComment(ctx, pixiv.StampArtworkCommentRequest{
				ArtworkID: in.ID,
				Comment:   in.Comment,
				StampID:   in.StampID,
			})
			if err != nil {
				return err
			}
			out.CommentID = result.CommentID
			return nil
		})
	})
}
