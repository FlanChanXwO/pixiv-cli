// Package reply_artwork_comment 实现 reply_artwork_comment tool。
package reply_artwork_comment

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 reply_artwork_comment。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:        "reply_artwork_comment",
		Description: "Reply to a comment on a Pixiv artwork.",
		InputSchema: schemas.ClosedObject(artworkCommentProperties(), []string{"illust_id", "comment", "parent_comment_id"}),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handleReplyArtworkComment(ctx, app, input)
	})
}

// In 的 parent_comment_id 是明确的上游父评论 ID，不通过评论列表推测。
type In struct {
	ArtworkID       int64  `json:"illust_id" jsonschema:"positive artwork ID"`
	Comment         string `json:"comment" jsonschema:"comment body"`
	ParentCommentID int64  `json:"parent_comment_id" jsonschema:"positive parent comment ID"`
}

func artworkCommentProperties() map[string]any {
	return map[string]any{
		"illust_id":         schemas.PositiveInteger("Positive Pixiv artwork ID."),
		"comment":           map[string]any{"type": "string", "minLength": 1, "description": "Non-empty comment body."},
		"parent_comment_id": schemas.PositiveInteger("Positive parent comment ID."),
	}
}

func handleReplyArtworkComment(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	out := outputs.Mutation{
		Action:   "reply_artwork_comment",
		IllustID: in.ArtworkID,
		Text:     fmt.Sprintf("Replied to comment %d on artwork %d.", in.ParentCommentID, in.ArtworkID),
	}
	return outputs.RunMutationInPlace(&out, func() error {
		return runtime.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			result, err := client.ReplyArtworkComment(ctx, pixiv.ReplyArtworkCommentRequest{
				ArtworkID:       in.ArtworkID,
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
