// Package bookmark_add 实现 pixiv_bookmark_add tool。
package bookmark_add

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_bookmark_add。
func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "pixiv_bookmark_add", Description: "Add an artwork or novel bookmark.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(map[string]any{"kind": schemas.EnumString("Target content type.", "artwork", "novel"), "id": schemas.PositiveInteger("Positive artwork or novel ID."), "restrict": map[string]any{"type": "string", "description": "public or private; omitted or empty uses SDK default."}, "tags": map[string]any{"type": []string{"array", "null"}, "items": map[string]any{"type": "string"}}}, []string{"kind", "id"})}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Mutation, error) {
		return handle(ctx, app, input)
	})
}

// In 要求显式选择目标类型；输出仍保留原操作的字段与语义。
type In struct {
	Kind     string   `json:"kind"`
	ID       int64    `json:"id" jsonschema:"artwork ID"`
	Restrict string   `json:"restrict,omitempty" jsonschema:"public or private"`
	Tags     []string `json:"tags,omitempty" jsonschema:"bookmark tags"`
}

func handle(ctx context.Context, app *toolhost.App, in In) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == "novel" {
		out := outputs.Mutation{Action: "add_novel_bookmark", NovelID: in.ID, Text: fmt.Sprintf("Bookmarked novel %d.", in.ID)}
		return outputs.RunMutation(out, func() error {
			return toolhost.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
				return client.AddNovelBookmark(ctx, pixiv.AddNovelBookmarkRequest{NovelID: in.ID, Restrict: pixiv.Restrict(in.Restrict), Tags: in.Tags})
			})
		})
	}

	out := outputs.Mutation{Action: "add_bookmark", IllustID: in.ID, Text: fmt.Sprintf("Bookmarked artwork %d.", in.ID)}
	return outputs.RunMutation(out, func() error {
		return toolhost.Write(app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.AddBookmark(ctx, pixiv.AddBookmarkRequest{ArtworkID: in.ID, Restrict: pixiv.Restrict(in.Restrict), Tags: in.Tags})
		})
	})
}
