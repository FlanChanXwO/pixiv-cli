package tools

import (
	"context"
	"errors"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerAccountList 注册 pixiv_account_list，只读本地账号；首次采用或显式选择会写 MCP 状态。
func (t pixivTools) registerAccountList(server *mcp.Server) {
	outputs.ProtectAccountInput(server, "pixiv_account_list")
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_account_list", Description: "Inspect local Pixiv accounts and selection without network refresh; may persist the initial explicit-default or sole-account selection.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, outputs.AccountStatus, error) {
		status, err := t.app.AccountSelection().Status(ctx)
		return outputs.AccountResult(status, err)
	})
}

type accountLoginStartIn struct {
	Restart bool `json:"restart,omitempty" jsonschema:"Cancel and replace the current login instead of reusing pending work"`
}

func (t pixivTools) registerAccountLoginStart(server *mcp.Server) {
	outputs.ProtectLoginInput(server, "pixiv_account_login_start")
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_account_login_start", Description: "Start or reuse a server-owned Pixiv login with the installed local pixiv:// helper. Returns a relay URL, not credentials. restart=true cancels the previous login. Completion saves the account, preserving any locally usable MCP selection; it selects the saved account only when no usable selection exists. The CLI default is unchanged.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(true), IdempotentHint: false, OpenWorldHint: new(true)}}, func(ctx context.Context, _ *mcp.CallToolRequest, in accountLoginStartIn) (*mcp.CallToolResult, outputs.LoginStatus, error) {
		manager := t.app.SDKPorts().Login
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

type accountStatusIn struct {
	LoginID *string `json:"login_id,omitempty" jsonschema:"Query the current or most recent login attempt without reading account storage"`
}

// registerAccountStatus 注册 pixiv_account_status；不带 login_id 时读本地状态，首次采用会写
// MCP 状态。
func (t pixivTools) registerAccountStatus(server *mcp.Server) {
	outputs.ProtectAccountInput(server, "pixiv_account_status")
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_account_status", Description: "Inspect local Pixiv accounts and selection, or query login progress with login_id. Without login_id, may persist the initial explicit-default or sole-account selection. Login queries remain available after storage failures.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}}, func(ctx context.Context, _ *mcp.CallToolRequest, in accountStatusIn) (*mcp.CallToolResult, outputs.AccountStatus, error) {
		if in.LoginID != nil {
			manager := t.app.SDKPorts().Login
			if manager == nil {
				return outputs.AccountResult(accounts.Status{}, errors.New("login unavailable"))
			}
			return outputs.LoginQuery(manager.Status(*in.LoginID))
		}
		status, err := t.app.AccountSelection().Status(ctx)
		return outputs.AccountResult(status, err)
	})
}

type accountUseIn struct {
	UserID int64 `json:"user_id" jsonschema:"positive local account user ID"`
}

// registerAccountUse 注册 pixiv_account_use，把显式选择写入共享的 MCP 账号状态。
func (t pixivTools) registerAccountUse(server *mcp.Server) {
	outputs.ProtectAccountInput(server, "pixiv_account_use")
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_account_use", Description: "Select an existing local Pixiv account for all connectors without changing the CLI default.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}}, func(ctx context.Context, _ *mcp.CallToolRequest, in accountUseIn) (*mcp.CallToolResult, outputs.AccountStatus, error) {
		status, err := t.app.AccountSelection().Use(ctx, in.UserID)
		return outputs.AccountResult(status, err)
	})
}
