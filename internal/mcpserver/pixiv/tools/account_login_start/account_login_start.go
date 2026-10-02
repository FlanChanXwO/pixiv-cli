// Package account_login_start starts the server-owned Pixiv helper login flow.
package account_login_start

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type input struct {
	Restart bool `json:"restart,omitempty" jsonschema:"Cancel and replace the current login instead of reusing pending work"`
}

func Register(app *runtime.App, server *mcp.Server) {
	outputs.ProtectLoginInput(server, "pixiv_account_login_start")
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_account_login_start", Description: "Start or reuse a server-owned Pixiv login with the installed local pixiv:// helper. Returns a relay URL, not credentials. restart=true cancels the previous login. Completion saves the account and selects it for MCP without changing the CLI default.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(true), IdempotentHint: false, OpenWorldHint: new(true)}}, func(ctx context.Context, _ *mcp.CallToolRequest, in input) (*mcp.CallToolResult, outputs.LoginStatus, error) {
		manager := app.SDKPorts().Login
		if manager == nil {
			return outputs.LoginResult(accounts.LoginStatus{}, errors.New("login unavailable"))
		}
		if err := ctx.Err(); err != nil {
			return outputs.LoginResult(accounts.LoginStatus{}, err)
		}
		status, err := manager.Start(in.Restart)
		return outputs.LoginResult(status, err)
	})
}
