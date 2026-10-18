// Package account_status exposes the owner's local Pixiv account status.
package account_status

import (
	"context"
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type input struct {
	LoginID *string `json:"login_id,omitempty" jsonschema:"Query the current or most recent login attempt without reading account storage"`
}

// Register adds pixiv_account_status; first-use adoption and explicit selection are local writes.
func Register(app *runtime.App, server *mcp.Server) {
	outputs.ProtectAccountInput(server, "pixiv_account_status")
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_account_status", Description: "Inspect local Pixiv accounts and selection, or query login progress with login_id. Without login_id, may persist the initial explicit-default or sole-account selection. Login queries remain available after storage failures.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}}, func(ctx context.Context, _ *mcp.CallToolRequest, in input) (*mcp.CallToolResult, outputs.AccountStatus, error) {
		if in.LoginID != nil {
			manager := app.SDKPorts().Login
			if manager == nil {
				return outputs.AccountResult(accounts.Status{}, errors.New("login unavailable"))
			}
			return outputs.LoginQuery(manager.Status(*in.LoginID))
		}
		status, err := app.AccountManager().Status(ctx)
		return outputs.AccountResult(status, err)
	})
}
