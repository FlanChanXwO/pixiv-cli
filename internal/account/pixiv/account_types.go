package pixiv

// AccountsResult 是账号列表。
type AccountsResult struct {
	Accounts  []Account `json:"accounts"`
	DefaultID int64     `json:"default_user_id,omitempty"`
}
