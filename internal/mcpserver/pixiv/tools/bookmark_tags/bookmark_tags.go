// Package bookmark_tags 实现 pixiv_bookmark_tags tool。
package bookmark_tags

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_bookmark_tags。
func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "pixiv_bookmark_tags", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "List artwork (default) or novel bookmark tags.", InputSchema: inputSchema(), OutputSchema: records.BookmarkTagsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.BookmarkTags, error) {
		return handleBookmarkTags(ctx, app, input)
	})
}

type In struct {
	Kind     string `json:"kind,omitempty"`
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	toolhost.PageLimitIn
}

func handleBookmarkTags(ctx context.Context, app *toolhost.App, in In) (*mcp.CallToolResult, outputs.BookmarkTags, error) {
	if in.Kind != "" && in.Kind != "artwork" && in.Kind != "novel" {
		return outputs.BookmarkTagsError(errors.New("kind must be artwork or novel"))
	}
	userID, err := toolhost.ResolveUserID(app, ctx, in.UserID)
	if err != nil {
		return outputs.BookmarkTagsError(err)
	}
	plan, err := toolhost.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.BookmarkTagsError(err)
	}
	items, more, err := toolhost.CollectWith(ctx, app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.BookmarkTag, sdk.Cursor, error) {
		if in.Kind == "novel" {
			result, err := client.UserNovelBookmarkTags(ctx, pixiv.UserNovelBookmarkTagsRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		result, err := client.UserArtworkBookmarkTags(ctx, pixiv.UserArtworkBookmarkTagsRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.BookmarkTagsError(err)
	}
	tags := make([]pixiv.BookmarkTagDTO, 0, len(items))
	for _, item := range items {
		tags = append(tags, pixiv.ToBookmarkTagDTO(item))
	}
	out := outputs.BookmarkTags{Tags: tags, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.BookmarkTagsResult(out, len(items)), out, nil
}

func inputSchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"kind":     schemas.EnumString("Content kind; defaults to artwork.", "artwork", "novel"),
			"user_id":  map[string]any{"type": "integer", "description": "optional user ID; defaults to authenticated user"},
			"restrict": map[string]any{"type": "string", "description": "public or private"},
			"page":     map[string]any{"type": []string{"null", "integer"}, "description": "1-based logical page; requires a positive limit"},
			"limit":    map[string]any{"type": []string{"null", "integer"}, "description": "maximum items; 0 returns all items"},
		},
	}
}
