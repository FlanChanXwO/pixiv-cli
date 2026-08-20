// Package resolveurl 实现 fanbox_resolve_url tool。
package resolveurl

import (
	"context"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_resolve_url。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "fanbox_resolve_url", Description: "Resolve a FANBOX page URL into a typed reference."}, func(ctx context.Context, request *mcp.CallToolRequest, input in) (*mcp.CallToolResult, out, error) {
		return handle(ctx, app, input)
	})
}

type in struct {
	URL string `json:"url" jsonschema:"required FANBOX page URL"`
}

type out struct {
	Kind      string `json:"kind"`
	CreatorID string `json:"creator_id,omitempty"`
	PostID    string `json:"post_id,omitempty"`
	Tag       string `json:"tag,omitempty"`
}

func handle(ctx context.Context, app *runtime.App, input in) (*mcp.CallToolResult, out, error) {
	result := out{}
	if strings.TrimSpace(input.URL) == "" {
		return runtime.Result(result, true, "Error: url is required"), result, nil
	}
	lease, err := app.OpenClient(ctx)
	if err != nil {
		return runtime.Result(result, true, "Error: "+err.Error()), result, nil
	}
	defer lease.Close()
	client := lease.Value()
	ref, err := client.ResolveURL(ctx, fanbox.ResolveURLRequest{RawURL: input.URL})
	if err != nil {
		return runtime.Result(result, true, "Error: "+err.Error()), result, nil
	}
	result = out{Kind: string(ref.Kind), CreatorID: ref.CreatorID, PostID: ref.PostID, Tag: ref.Tag}
	return runtime.Result(result, false, fmt.Sprintf("Resolved URL as %s.", result.Kind)), result, nil
}
