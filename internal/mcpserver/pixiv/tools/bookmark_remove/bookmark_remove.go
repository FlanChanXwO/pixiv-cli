// Package bookmark_remove 实现 pixiv_bookmark_remove tool。
package bookmark_remove

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_bookmark_remove。
func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "pixiv_bookmark_remove", Description: "Remove an artwork or novel bookmark.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(map[string]any{"kind": schemas.EnumString("Target content type.", "artwork", "novel"), "id": schemas.PositiveInteger("Positive artwork or novel ID.")}, []string{"kind", "id"})}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handle(ctx, app, input)
	})
}

// In 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type In struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id" jsonschema:"artwork ID"`
}

func handle(ctx context.Context, app *toolhost.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == "novel" {
		out := outputs.Mutation{Action: "remove_novel_bookmark", NovelID: in.ID, Text: fmt.Sprintf("Removed bookmark from novel %d.", in.ID)}
		return outputs.RunMutation(out, func() error {
			return toolhost.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
				return client.RemoveNovelBookmark(ctx, pixiv.RemoveNovelBookmarkRequest{NovelID: in.ID})
			})
		})
	}

	out := outputs.Mutation{Action: "remove_bookmark", IllustID: in.ID, Text: fmt.Sprintf("Removed bookmark from artwork %d.", in.ID)}
	return outputs.RunMutation(out, func() error {
		return toolhost.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.RemoveArtworkBookmark(ctx, pixiv.RemoveArtworkBookmarkRequest{ArtworkID: in.ID})
		})
	})
}
