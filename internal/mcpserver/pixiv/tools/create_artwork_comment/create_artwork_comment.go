// Package create_artwork_comment 实现 create_artwork_comment tool。
package create_artwork_comment

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 create_artwork_comment。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:        "create_artwork_comment",
		Description: "Create a top-level comment on a Pixiv artwork.",
		InputSchema: schemas.ClosedObject(artworkCommentProperties(), []string{"illust_id", "comment"}),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleCreateArtworkComment(ctx, app, input)
	})
}

// In 只接受明确的作品 ID 与评论正文；创建 ID 必须来自上游响应。
type In struct {
	ArtworkID int64  `json:"illust_id" jsonschema:"positive artwork ID"`
	Comment   string `json:"comment" jsonschema:"comment body"`
}

func artworkCommentProperties() map[string]any {
	return map[string]any{
		"illust_id": schemas.PositiveInteger("Positive Pixiv artwork ID."),
		"comment":   map[string]any{"type": "string", "minLength": 1, "description": "Non-empty comment body."},
	}
}

func handleCreateArtworkComment(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{
		Action:   "create_artwork_comment",
		IllustID: in.ArtworkID,
		Text:     fmt.Sprintf("Created comment on artwork %d.", in.ArtworkID),
	}
	return outputs.RunMutationInPlace(&out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			result, err := client.PostArtworkComment(ctx, pixiv.PostArtworkCommentRequest{
				ArtworkID: in.ArtworkID,
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
