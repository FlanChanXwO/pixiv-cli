// Package comment_create 实现 pixiv_comment_create tool。
package comment_create

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_comment_create。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_comment_create", Description: "Create a top-level comment on an artwork or novel.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: false, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(commentProperties(), []string{"kind", "id", "comment"})}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handle(ctx, app, input)
	})
}

// In 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type In struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id" jsonschema:"positive artwork or novel ID"`
	Comment string `json:"comment" jsonschema:"comment body"`
}

func commentProperties() map[string]any {
	return map[string]any{
		"kind":    schemas.EnumString("Target content type.", "artwork", "novel"),
		"id":      schemas.PositiveInteger("Positive Pixiv artwork or novel ID."),
		"comment": map[string]any{"type": "string", "minLength": 1, "description": "Non-empty comment body."},
	}
}

func handle(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == "novel" {
		out := outputs.Mutation{
			Action:  "create_novel_comment",
			NovelID: in.ID,
			Text:    fmt.Sprintf("Created comment on novel %d.", in.ID),
		}
		return outputs.RunMutationInPlace(&out, func() error {
			return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
				result, err := client.PostNovelComment(ctx, pixiv.PostNovelCommentRequest{
					NovelID: in.ID,
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

	out := outputs.Mutation{
		Action:   "create_artwork_comment",
		IllustID: in.ID,
		Text:     fmt.Sprintf("Created comment on artwork %d.", in.ID),
	}
	return outputs.RunMutationInPlace(&out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			result, err := client.PostArtworkComment(ctx, pixiv.PostArtworkCommentRequest{
				ArtworkID: in.ID,
				Comment:   in.Comment,
			})
			if err != nil {
				return err
			}
			out.CommentID = result.CommentID
			return nil
		})
	})
}
