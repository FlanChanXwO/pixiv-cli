// Package stamp_artwork_comment 实现 stamp_artwork_comment tool。
package stamp_artwork_comment

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 stamp_artwork_comment。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:        "stamp_artwork_comment",
		Description: "Add a stamp comment to a Pixiv artwork.",
		InputSchema: schemas.ClosedObject(artworkCommentProperties(), []string{"illust_id", "comment", "stamp_id"}),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleStampArtworkComment(ctx, app, input)
	})
}

// In 把 stamp_id 作为独立的上游字段传递，不把它当作回复父评论 ID。
type In struct {
	ArtworkID int64  `json:"illust_id" jsonschema:"positive artwork ID"`
	Comment   string `json:"comment" jsonschema:"comment body"`
	StampID   int64  `json:"stamp_id" jsonschema:"positive stamp ID"`
}

func artworkCommentProperties() map[string]any {
	return map[string]any{
		"illust_id": schemas.PositiveInteger("Positive Pixiv artwork ID."),
		"comment":   map[string]any{"type": "string", "minLength": 1, "description": "Non-empty comment body."},
		"stamp_id":  schemas.PositiveInteger("Positive Pixiv stamp ID."),
	}
}

func handleStampArtworkComment(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{
		Action:   "stamp_artwork_comment",
		IllustID: in.ArtworkID,
		Text:     fmt.Sprintf("Added stamp comment to artwork %d.", in.ArtworkID),
	}
	return outputs.RunMutationInPlace(&out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			result, err := client.StampArtworkComment(ctx, pixiv.StampArtworkCommentRequest{
				ArtworkID: in.ArtworkID,
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
