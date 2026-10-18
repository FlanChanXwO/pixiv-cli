// Package account_use exposes the owner's local Pixiv account selection.
package account_use

import (
	"context"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type input struct {
	UserID int64 `json:"user_id" jsonschema:"positive local account user ID"`
}

// Register adds pixiv_account_use; first-use adoption and explicit selection are local writes.
func Register(app *runtime.App, server *mcp.Server) {
	outputs.ProtectAccountInput(server, "pixiv_account_use")
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_account_use", Description: "Select an existing local Pixiv account for all connectors without changing the CLI default.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}}, func(ctx context.Context, _ *mcp.CallToolRequest, in input) (*mcp.CallToolResult, outputs.AccountStatus, error) {
		status, err := app.AccountManager().Use(ctx, in.UserID)
		return outputs.AccountResult(status, err)
	})
}
