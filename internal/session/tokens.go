package session

import "github.com/FlanChanXwO/pixiv-cli/internal/credential"

// AccountWithToken 携带仅用于显式 auth export 的账号+token 摘要。
// RefreshToken 供显式导出流程读取凭据；通用 JSON 编码忽略 Credential。
type AccountWithToken struct {
	UserID     int64             `json:"user_id"`
	Username   string            `json:"username"`
	Default    bool              `json:"default"`
	Credential credential.Secret `json:"-"`
}

// RefreshToken 返回仅供显式 auth export 使用的 refresh token。
func (a AccountWithToken) RefreshToken() string { return a.Credential.Value() }
