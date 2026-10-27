package outputs

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AccountStatus 承载账号工具的输出；保存失败时仍保留本地选择状态，并把安全错误码写进
// Error。
type AccountStatus struct {
	accounts.Status
	Login *accounts.LoginStatus `json:"login,omitempty"`
	Error string                `json:"error,omitempty"`
}

// AccountResult 把错误映射为稳定的本地错误类别，输出里不出现数据库路径或底层原因。
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
	out := AccountStatus{Status: accounts.Status{SelectionState: accounts.SelectionUnknown, CredentialState: accounts.CredentialUnknown, Accounts: []accounts.Account{}}, Error: code}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: code}}}, out, nil
}

// ProtectAccountInput 把 SDK 的输入校验诊断替换成固定文案，避免回显原始参数。typed
// handler 已经执行时不会替换，其存储错误保持原样。
func ProtectAccountInput(server *mcp.Server, name string) {
	toolrun.ProtectInput(server, name, func() *mcp.CallToolResult {
		safe, out, _ := accountFailure("invalid_request")
		safe.StructuredContent = out
		return safe
	})
}
