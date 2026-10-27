package outputs

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LoginStatus 承载登录工具的输出，只包含接力入口 URL，不含 SDK 授权 URL 与校验材料。
type LoginStatus struct {
	accounts.LoginStatus
	RequiresLocalHelper bool   `json:"requires_local_helper"`
	Instructions        string `json:"instructions"`
	Error               string `json:"error,omitempty"`
}

// LoginResult 把开始登录的结果包成 MCP 输出；失败时只返回 login_start_failed。
func LoginResult(status accounts.LoginStatus, err error) (*mcp.CallToolResult, LoginStatus, error) {
	out := LoginStatus{LoginStatus: status, RequiresLocalHelper: true, Instructions: "Open authorization_url on a computer with pixiv-cli installed and its pixiv:// login helper registered. If the helper does not open, install or reinstall pixiv-cli with the official installer and reopen the URL. Check pixiv_account_status with login_id for progress; do not paste callback URLs or credentials into chat."}
	if status.Status == "exchanging" {
		out.Instructions = "Login is already completing. Query pixiv_account_status with login_id for progress; do not reopen the helper or paste callback URLs into chat."
	}
	if err != nil {
		out = LoginStatus{LoginStatus: accounts.LoginStatus{Status: "failed"}, Error: "login_start_failed"}
	}
	return &mcp.CallToolResult{IsError: err != nil || out.Status == "failed", Content: []mcp.Content{&mcp.TextContent{Text: out.Status}}}, out, nil
}

// LoginQuery 只报告登录进度，不读账号存储；选择写入失败时查询仍然可用。
func LoginQuery(status accounts.LoginStatus) (*mcp.CallToolResult, AccountStatus, error) {
	out := AccountStatus{Status: accounts.Status{SelectionState: accounts.SelectionUnknown, CredentialState: accounts.CredentialUnknown, Accounts: []accounts.Account{}}, Login: &status}
	return &mcp.CallToolResult{IsError: status.Status == "failed", Content: []mcp.Content{&mcp.TextContent{Text: status.Status}}}, out, nil
}

// ProtectLoginInput 把输入校验错误转成登录工具声明的 envelope，避免 SDK 原始诊断外泄。
func ProtectLoginInput(server *mcp.Server, name string) {
	toolrun.ProtectInput(server, name, func() *mcp.CallToolResult {
		out := LoginStatus{LoginStatus: accounts.LoginStatus{Status: "failed"}, Error: "invalid_request"}
		return &mcp.CallToolResult{IsError: true, StructuredContent: out, Content: []mcp.Content{&mcp.TextContent{Text: "invalid_request"}}}
	})
}
