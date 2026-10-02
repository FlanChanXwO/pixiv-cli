package outputs

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LoginStatus contains a relay entry URL only; SDK authorization URLs and proof are excluded from MCP output.
type LoginStatus struct {
	accounts.LoginStatus
	RequiresLocalHelper bool   `json:"requires_local_helper"`
	Instructions        string `json:"instructions"`
	Error               string `json:"error,omitempty"`
}

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

// LoginQuery does not read account storage: even a failed selection write must remain queryable.
func LoginQuery(status accounts.LoginStatus) (*mcp.CallToolResult, AccountStatus, error) {
	out := AccountStatus{Status: accounts.Status{SelectionState: "unknown", CredentialState: "unknown", Accounts: []accounts.Account{}}, Login: &status}
	return &mcp.CallToolResult{IsError: status.Status == "failed", Content: []mcp.Content{&mcp.TextContent{Text: status.Status}}}, out, nil
}

// ProtectLoginInput keeps validation errors within the advertised login envelope.
func ProtectLoginInput(server *mcp.Server, name string) {
	protectInput(server, name, func() *mcp.CallToolResult {
		out := LoginStatus{LoginStatus: accounts.LoginStatus{Status: "failed"}, Error: "invalid_request"}
		return &mcp.CallToolResult{IsError: true, StructuredContent: out, Content: []mcp.Content{&mcp.TextContent{Text: "invalid_request"}}}
	})
}
