// Package account_list exposes the owner's local Pixiv account status.
package account_list

import (
	"context"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register adds pixiv_account_list; first-use adoption and explicit selection are local writes.
func Register(app *runtime.App, server *mcp.Server) {
	outputs.ProtectAccountInput(server, "pixiv_account_list")
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_account_list", Description: "Inspect local Pixiv accounts and selection without network refresh; may persist the initial explicit-default or sole-account selection.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, outputs.AccountStatus, error) {
		status, err := app.AccountManager().Status(ctx)
		return outputs.AccountResult(status, err)
	})
}
