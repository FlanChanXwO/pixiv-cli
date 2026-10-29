package account

import "github.com/FlanChanXwO/pixiv-cli/internal/credential"

// AccountWithToken 携带仅用于显式 auth export 的账号+token 摘要。
// RefreshToken 供显式导出流程读取凭据；通用 JSON 编码忽略 Credential。
type AccountWithToken struct {
	UserID     int64             `json:"user_id"`
	Username   string            `json:"username"`
	Default    bool              `json:"default"`
	Credential credential.Secret `json:"-"`
}

// newAccountWithToken 创建一个带有 opaque refresh token 的导出值。
// 调用方只有在明确的导出流程中通过 RefreshToken 读取 token。
func newAccountWithToken(userID int64, username string, isDefault bool, refreshToken string) AccountWithToken {
	return AccountWithToken{
		UserID:     userID,
		Username:   username,
		Default:    isDefault,
		Credential: credential.New([]byte(refreshToken)),
	}
}

// RefreshToken 返回仅供显式 auth export 使用的 refresh token。
func (a AccountWithToken) RefreshToken() string { return a.Credential.Value() }
