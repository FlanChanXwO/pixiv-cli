// Package gallery_animation owns short control requests for Gallery animations.
package gallery_animation

import (
	"context"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/mediaproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type input struct {
	Action   string `json:"action"`
	IllustID int64  `json:"illust_id,omitempty"`
	Format   string `json:"format,omitempty"`
	Handle   string `json:"handle,omitempty"`
}

func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_gallery_animation", Description: "Prepare, inspect or release a Gallery animation subscription. Short control responses contain protected animation and PNG preview URLs when ready. Poll status every 2 seconds; failures require explicit retry. Existing pixiv_artwork_media delivers full embedded media.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), OpenWorldHint: new(true)}, InputSchema: schemas.ClosedObject(map[string]any{"action": schemas.EnumString("Operation", "prepare", "status", "release"), "illust_id": schemas.PositiveInteger("Artwork ID for prepare"), "format": schemas.EnumString("Prepare format, default gif", "gif", "apng"), "handle": map[string]any{"type": "string", "minLength": 1}}, []string{"action"})}, func(ctx context.Context, _ *mcp.CallToolRequest, in input) (*mcp.CallToolResult, mediaproxy.AnimationStatus, error) {
		out := mediaproxy.AnimationStatus{State: "failed"}
		fail := func(code string) (*mcp.CallToolResult, mediaproxy.AnimationStatus, error) {
			out.State = "failed"
			out.Error = code
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: code}}}, out, nil
		}
		if in.Action == "prepare" {
			if in.IllustID <= 0 || in.Handle != "" || (in.Format != "" && in.Format != "gif" && in.Format != "apng") {
				return fail("invalid_request")
			}
		} else if (in.Action != "status" && in.Action != "release") || in.Handle == "" || in.IllustID != 0 || in.Format != "" {
			return fail("invalid_request")
		}
		media := app.SDKPorts().Media
		if media == nil {
			return fail("animation_service_unavailable")
		}
		account, err := app.ResolveAccount(ctx)
		if err != nil {
			return fail("account_unavailable")
		}
		if in.Action == "prepare" {
			out, err = media.PrepareAnimation(ctx, account.UserID, in.IllustID, in.Format)
		} else {
			out, err = media.AnimationSubscription(ctx, account.UserID, in.Handle, in.Action == "release")
		}
		if err != nil {
			return fail("animation_unavailable")
		}
		return &mcp.CallToolResult{IsError: out.State == "failed", Content: []mcp.Content{&mcp.TextContent{Text: "Animation " + out.State}}}, out, nil
	})
}
