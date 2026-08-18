// Package creatortags 实现 fanbox_creator_tags tool。
package creatortags

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_creator_tags。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "fanbox_creator_tags", Description: "List tags used by a FANBOX creator."}, func(ctx context.Context, request *mcp.CallToolRequest, input in) (*mcp.CallToolResult, out, error) {
		return handle(ctx, app, input)
	})
}

type in struct {
	CreatorID string `json:"creator_id" jsonschema:"required FANBOX creator id"`
}

type tagOut struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type out struct {
	Tags []tagOut `json:"tags"`
}

func handle(ctx context.Context, app *runtime.App, input in) (*mcp.CallToolResult, out, error) {
	out := out{Tags: []tagOut{}}
	if input.CreatorID == "" {
		return runtime.Result(out, true, "Error: creator_id is required"), out, nil
	}
	lease, err := app.OpenClient(ctx)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer lease.Close()
	client := lease.Value()
	tags, err := client.CreatorTags(ctx, fanbox.CreatorTagsRequest{CreatorID: input.CreatorID})
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	for _, tag := range tags {
		out.Tags = append(out.Tags, tagOut{Name: tag.Name, URL: tag.URL})
	}
	return runtime.Result(out, false, fmt.Sprintf("Retrieved %d tags.", len(out.Tags))), out, nil
}
