package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t pixivTools) registerCommentCreate(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_comment_create", Description: "Create a top-level comment on an artwork or novel.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: false, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(commentCreateCommentProperties(), []string{"kind", "id", "comment"})}, func(ctx context.Context, request *mcp.CallToolRequest, input commentCreateIn) (*mcp.CallToolResult, outputs.Mutation, error) {
		return t.commentCreateHandler(ctx, input)
	})
}

// commentCreateIn 的 kind 决定评论作品还是小说；输出沿用评论 mutation 的 envelope。
type commentCreateIn struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id" jsonschema:"positive artwork or novel ID"`
	Comment string `json:"comment" jsonschema:"comment body"`
}

func commentCreateCommentProperties() map[string]any {
	return map[string]any{
		"kind":    schemas.EnumString("Target content type.", kindArtwork, kindNovel),
		"id":      schemas.PositiveInteger("Positive Pixiv artwork or novel ID."),
		"comment": map[string]any{"type": "string", "minLength": 1, "description": "Non-empty comment body."},
	}
}

func (t pixivTools) commentCreateHandler(ctx context.Context, in commentCreateIn) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == kindNovel {
		out := outputs.Mutation{
			Action:  "create_novel_comment",
			NovelID: in.ID,
			Text:    fmt.Sprintf("Created comment on novel %d.", in.ID),
		}
		return outputs.RunMutation(&out, func() error {
			return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
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
	return outputs.RunMutation(&out, func() error {
		return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
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
func (t pixivTools) registerCommentDelete(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_comment_delete", Description: "Delete an artwork or novel comment by comment ID.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(map[string]any{"kind": schemas.EnumString("Target content type.", kindArtwork, kindNovel), "comment_id": schemas.PositiveInteger("Positive Pixiv comment ID.")}, []string{"kind", "comment_id"})}, func(ctx context.Context, request *mcp.CallToolRequest, input commentDeleteIn) (*mcp.CallToolResult, outputs.Mutation, error) {
		return t.commentDeleteHandler(ctx, input)
	})
}

// commentDeleteIn 的 kind 决定删除作品还是小说下的评论。
type commentDeleteIn struct {
	Kind      string `json:"kind"`
	CommentID int64  `json:"comment_id" jsonschema:"positive comment ID"`
}

func (t pixivTools) commentDeleteHandler(ctx context.Context, in commentDeleteIn) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == kindNovel {
		out := outputs.Mutation{
			Action:    "delete_novel_comment",
			CommentID: in.CommentID,
			Text:      fmt.Sprintf("Deleted comment %d.", in.CommentID),
		}
		return outputs.RunMutation(&out, func() error {
			return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
				return client.DeleteNovelComment(ctx, pixiv.DeleteNovelCommentRequest{CommentID: in.CommentID})
			})
		})
	}

	out := outputs.Mutation{
		Action:    "delete_artwork_comment",
		CommentID: in.CommentID,
		Text:      fmt.Sprintf("Deleted comment %d.", in.CommentID),
	}
	return outputs.RunMutation(&out, func() error {
		return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.DeleteArtworkComment(ctx, pixiv.DeleteArtworkCommentRequest{CommentID: in.CommentID})
		})
	})
}
func (t pixivTools) registerCommentReply(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_comment_reply", Description: "Reply to an artwork or novel comment.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: false, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(commentReplyCommentProperties(), []string{"kind", "id", "comment", "parent_comment_id"})}, func(ctx context.Context, request *mcp.CallToolRequest, input commentReplyIn) (*mcp.CallToolResult, outputs.Mutation, error) {
		return t.commentReplyHandler(ctx, input)
	})
}

// commentReplyIn 的 kind 决定评论所属的作品或小说，parent_comment_id 为被回复的评论。
type commentReplyIn struct {
	Kind            string `json:"kind"`
	ID              int64  `json:"id" jsonschema:"positive artwork or novel ID"`
	Comment         string `json:"comment" jsonschema:"comment body"`
	ParentCommentID int64  `json:"parent_comment_id" jsonschema:"positive parent comment ID"`
}

func commentReplyCommentProperties() map[string]any {
	return map[string]any{
		"kind":              schemas.EnumString("Target content type.", kindArtwork, kindNovel),
		"id":                schemas.PositiveInteger("Positive Pixiv artwork or novel ID."),
		"comment":           map[string]any{"type": "string", "minLength": 1, "description": "Non-empty comment body."},
		"parent_comment_id": schemas.PositiveInteger("Positive parent comment ID."),
	}
}

func (t pixivTools) commentReplyHandler(ctx context.Context, in commentReplyIn) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == kindNovel {
		out := outputs.Mutation{
			Action:  "reply_novel_comment",
			NovelID: in.ID,
			Text:    fmt.Sprintf("Replied to comment %d on novel %d.", in.ParentCommentID, in.ID),
		}
		return outputs.RunMutation(&out, func() error {
			return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
				result, err := client.ReplyNovelComment(ctx, pixiv.ReplyNovelCommentRequest{
					NovelID:         in.ID,
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

	out := outputs.Mutation{
		Action:   "reply_artwork_comment",
		IllustID: in.ID,
		Text:     fmt.Sprintf("Replied to comment %d on artwork %d.", in.ParentCommentID, in.ID),
	}
	return outputs.RunMutation(&out, func() error {
		return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			result, err := client.ReplyArtworkComment(ctx, pixiv.ReplyArtworkCommentRequest{
				ArtworkID:       in.ID,
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
func (t pixivTools) registerCommentStamp(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_comment_stamp", Description: "Add a stamp comment to an artwork or novel.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: false, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(commentStampCommentProperties(), []string{"kind", "id", "stamp_id"})}, func(ctx context.Context, request *mcp.CallToolRequest, input commentStampIn) (*mcp.CallToolResult, outputs.Mutation, error) {
		return t.commentStampHandler(ctx, input)
	})
}

// commentStampIn 的 kind 决定贴图落在作品还是小说下；stamp_id 是 Pixiv 的贴图编号。
type commentStampIn struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id" jsonschema:"positive artwork or novel ID"`
	Comment string `json:"comment" jsonschema:"comment body"`
	StampID int64  `json:"stamp_id" jsonschema:"positive stamp ID"`
}

func commentStampCommentProperties() map[string]any {
	return map[string]any{
		"kind":     schemas.EnumString("Target content type.", kindArtwork, kindNovel),
		"id":       schemas.PositiveInteger("Positive Pixiv artwork or novel ID."),
		"comment":  map[string]any{"type": "string", "description": "Optional comment body; empty is the sticker-only wire form."},
		"stamp_id": schemas.PositiveInteger("Positive Pixiv stamp ID."),
	}
}

func (t pixivTools) commentStampHandler(ctx context.Context, in commentStampIn) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == kindNovel {
		out := outputs.Mutation{
			Action:  "stamp_novel_comment",
			NovelID: in.ID,
			Text:    fmt.Sprintf("Added stamp comment to novel %d.", in.ID),
		}
		return outputs.RunMutation(&out, func() error {
			return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
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
	return outputs.RunMutation(&out, func() error {
		return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
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
func (t pixivTools) registerComments(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_comments", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Read comments for a Pixiv artwork or novel.", InputSchema: commentsInputSchema(),
		OutputSchema: schemas.CommentOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input commentsIn) (*mcp.CallToolResult, outputs.Comments, error) {
		return t.commentsHandler(ctx, input)
	})
}

// commentsIn 的 kind 决定读取作品还是小说的评论。
type commentsIn struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id" jsonschema:"positive artwork or novel ID"`
	toolrun.ListIn
}

func (t pixivTools) commentsHandler(ctx context.Context, in commentsIn) (*mcp.CallToolResult, outputs.Comments, error) {
	if in.ID <= 0 {
		result, out, _ := outputs.CommentsError(errors.New("id must be a positive integer"))
		return result, out, nil
	}
	plan, err := toolrun.ParseListPlan(in.ListIn)
	if err != nil {
		result, out, _ := outputs.CommentsError(err)
		return result, out, nil
	}
	metadata := commentMetadata{}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Comment, sdk.Cursor, error) {
		var page pixiv.CommentPage
		var err error
		if in.Kind == kindNovel {
			page, err = client.NovelComments(ctx, pixiv.NovelCommentsRequest{NovelID: in.ID, Cursor: cursor})
		} else {
			page, err = client.ArtworkComments(ctx, pixiv.ArtworkCommentsRequest{ArtworkID: in.ID, Cursor: cursor})
		}
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		if cursor.IsZero() {
			metadata = commentMetadata{}
		}
		metadata.Set(page)
		return page.Page.Items, page.Page.Next, nil
	})
	if err != nil {
		result, out, _ := outputs.CommentsError(err)
		return result, out, nil
	}
	if items == nil {
		items = []pixiv.Comment{}
	}
	out := outputs.Comments{Comments: items, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more), Total: metadata.Total, AccessControl: metadata.Access}
	return outputs.CommentsResult(out, false, ""), out, nil
}

func commentsInputSchema() map[string]any {
	schema := schemas.CommentInputSchema()
	schema["properties"].(map[string]any)["kind"] = schemas.EnumString("Target content type.", kindArtwork, kindNovel)
	schema["required"] = []string{"kind", "id"}
	return schema
}

// commentMetadata 只记录上游明确给出的评论总数与访问控制，缺字段时不补默认值。
type commentMetadata struct {
	Total  *int64
	Access *pixiv.CommentAccessControl
}

// Set 只采用评论页里第一个出现的总数与访问控制，后续页不再覆盖。
func (m *commentMetadata) Set(page pixiv.CommentPage) {
	if m.Total == nil && page.Total != nil {
		m.Total = page.Total
	}
	if m.Access == nil && page.AccessControl != nil {
		m.Access = page.AccessControl
	}
}
