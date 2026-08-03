package application

// Account 是 CLI/MCP 展示的本地 Pixiv 账号摘要，不携带任何 secret。
type Account struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Default  bool   `json:"default"`
	Premium  *bool  `json:"premium,omitempty"`
}

// AccountsResult 是账号列表。
type AccountsResult struct {
	Accounts  []Account `json:"accounts"`
	DefaultID int64     `json:"default_user_id,omitempty"`
}
