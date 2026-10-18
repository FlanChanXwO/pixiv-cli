// Package feed 实现 fanbox_feed tool。
package feed

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_feed。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "fanbox_feed", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, InputSchema: inputSchema(), Description: "Browse the FANBOX home or supporting feed."}, func(ctx context.Context, request *mcp.CallToolRequest, input in) (*mcp.CallToolResult, runtime.PostsOut, error) {
		return handle(ctx, app, input)
	})
}

func handle(ctx context.Context, app *runtime.App, input in) (*mcp.CallToolResult, runtime.PostsOut, error) {
	out := runtime.PostsOut{Posts: []runtime.PostOut{}}
	plan, err := runtime.ParseListPlan(input.ListIn)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	if input.Source != "home" && input.Source != "supporting" {
		return runtime.Result(out, true, "Error: source must be home or supporting"), out, nil
	}
	lease, err := app.OpenClient(ctx)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer lease.Close()
	client := lease.Value()
	return runtime.PostList(ctx, app, client, input.Limit, plan, func(ctx context.Context, client *fanbox.Client, cursor sdk.Cursor) (sdk.Page[fanbox.Post], error) {
		if input.Source == "supporting" {
			return client.Supporting(ctx, fanbox.SupportingRequest{Cursor: cursor})
		}
		return client.Home(ctx, fanbox.HomeRequest{Cursor: cursor})
	})
}

type in struct {
	Source string `json:"source" jsonschema:"required: home or supporting"`
	runtime.ListIn
}

func inputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[in](nil)
	if err != nil {
		panic(err)
	}
	schema.Properties["source"].Enum = []any{"home", "supporting"}
	return schema
}
