package outputs

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AccountStatus preserves the local status envelope even when account storage fails.
type AccountStatus struct {
	accounts.Status
	Login *accounts.LoginStatus `json:"login,omitempty"`
	Error string                `json:"error,omitempty"`
}

// AccountResult only exposes stable local error categories, never database paths or causes.
func AccountResult(status accounts.Status, err error) (*mcp.CallToolResult, AccountStatus, error) {
	out := AccountStatus{Status: status}
	if err != nil {
		code := "local_state_error"
		switch {
		case errors.Is(err, accounts.ErrInvalidUserID):
			code = "invalid_user_id"
		case errors.Is(err, accounts.ErrAccountNotFound):
			code = "account_not_found"
		case errors.Is(err, auth.ErrNotInitialized):
			code = "owner_not_initialized"
		case errors.Is(err, context.Canceled):
			code = "canceled"
		case errors.Is(err, context.DeadlineExceeded):
			code = "deadline_exceeded"
		}
		return accountFailure(code)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: status.SelectionState}}}, out, nil
}

func accountFailure(code string) (*mcp.CallToolResult, AccountStatus, error) {
	out := AccountStatus{Status: accounts.Status{SelectionState: "unknown", CredentialState: "unknown", Accounts: []accounts.Account{}}, Error: code}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: code}}}, out, nil
}

// ProtectAccountInput replaces SDK validation diagnostics that otherwise echo raw arguments.
// Typed handlers always return structured content, so their storage errors remain intact.
func ProtectAccountInput(server *mcp.Server, name string) {
	protectInput(server, name, func() *mcp.CallToolResult {
		safe, out, _ := accountFailure("invalid_request")
		safe.StructuredContent = out
		return safe
	})
}

func protectInput(server *mcp.Server, name string, failure func() *mcp.CallToolResult) {
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, request)
			call, ok := request.(*mcp.CallToolRequest)
			if !ok || call.Params.Name != name {
				return result, err
			}
			failed, ok := result.(*mcp.CallToolResult)
			if !ok || !failed.IsError || failed.StructuredContent != nil {
				return result, err
			}
			return failure(), nil
		}
	})
}
