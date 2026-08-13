// Package openresource 实现 fanbox_open_resource tool。
package openresource

import (
	"context"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 fanbox_open_resource。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "fanbox_open_resource", Description: "Open a FANBOX media resource by ref and return its safe metadata and status without the bytes."}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		return handle(ctx, app, input)
	})
}

type In struct {
	Ref    string `json:"ref" jsonschema:"opaque FANBOX media resource reference"`
	Method string `json:"method,omitempty" jsonschema:"GET or HEAD; default GET"`
}

type Out struct {
	Ref           string `json:"ref"`
	StatusCode    int    `json:"status_code"`
	ContentType   string `json:"content_type,omitempty"`
	ContentLength int64  `json:"content_length,omitempty"`
}

func handle(ctx context.Context, app *runtime.App, input In) (*mcp.CallToolResult, Out, error) {
	out := Out{Ref: input.Ref}
	ref, err := sdk.ParseResourceRef(input.Ref)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	method := sdk.ResourceMethod(strings.ToUpper(input.Method))
	switch method {
	case "", sdk.ResourceMethodGet, sdk.ResourceMethodHead:
	default:
		return runtime.Result(out, true, `Error: method supports only "GET" or "HEAD"`), out, nil
	}
	client, err := app.OpenClient(ctx)
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	defer client.CloseIdleConnections()
	response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: ref, Method: method})
	if err != nil {
		return runtime.Result(out, true, "Error: "+err.Error()), out, nil
	}
	// 只返回 URL 消费所需的安全 metadata 与 status，绝不把字节返回给调用方。
	defer response.Body.Close()
	out.StatusCode = response.StatusCode
	out.ContentType = response.ContentType()
	out.ContentLength = response.ContentLength()
	return runtime.Result(out, false, fmt.Sprintf("Resource status %d.", response.StatusCode)), out, nil
}
