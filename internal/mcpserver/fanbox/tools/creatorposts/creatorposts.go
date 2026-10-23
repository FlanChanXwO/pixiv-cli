// Package creatorposts 实现 fanbox_creator_posts tool。
package creatorposts

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_creator_posts。
func Register(app *toolhost.App, server *mcp.Server) {
	toolhost.AddTool(app, server, &mcp.Tool{Name: "fanbox_creator_posts", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, InputSchema: inputSchema(), Description: "List posts from a FANBOX creator, optionally for one non-empty tag."}, func(ctx context.Context, request *mcp.CallToolRequest, input in) (*mcp.CallToolResult, toolhost.PostsOut, error) {
		return handle(ctx, app, input)
	})
}

type in struct {
	Tag       *string `json:"tag,omitempty" jsonschema:"optional non-empty tag"`
	CreatorID string  `json:"creator_id" jsonschema:"required FANBOX creator id"`
	toolhost.ListIn
}

func handle(ctx context.Context, app *toolhost.App, input in) (*mcp.CallToolResult, toolhost.PostsOut, error) {
	out := toolhost.PostsOut{Posts: []toolhost.PostOut{}}
	plan, err := toolhost.ParseListPlan(input.ListIn)
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	if input.CreatorID == "" {
		return toolhost.Result(out, true, "Error: creator_id is required"), out, nil
	}
	if input.Tag != nil && *input.Tag == "" {
		return toolhost.Result(out, true, "Error: tag must be non-empty"), out, nil
	}
	lease, err := app.OpenClient(ctx)
	if err != nil {
		return toolhost.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer lease.Close()
	client := lease.Value()
	creatorID := input.CreatorID
	return toolhost.PostList(ctx, app, client, input.Limit, plan, func(ctx context.Context, client *fanbox.Client, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		if input.Tag != nil {
			return client.TaggedPosts(ctx, fanbox.TaggedPostsRequest{CreatorID: creatorID, Tag: *input.Tag, Cursor: cursor})
		}
		return client.CreatorPosts(ctx, fanbox.CreatorPostsRequest{CreatorID: creatorID, Cursor: cursor})
	})
}

func inputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[in](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["tag"] = &jsonschema.Schema{Type: "string", Description: "Optional non-empty creator tag."}
	return schema
}
