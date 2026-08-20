package pixiv

import (
	"encoding/json"
	"fmt"
	"io"
)

// AccountWithToken 是仅用于显式 auth export 的账号+token 摘要。
// token 保持私有，只有显式导出流程可以通过 RefreshToken 读取。
type AccountWithToken struct {
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	Default      bool   `json:"default"`
	refreshToken string
}

// NewAccountWithToken 创建一个带有 opaque refresh token 的导出值。
// 调用方只有在明确的导出流程中通过 RefreshToken 读取 token。
func NewAccountWithToken(userID int64, username string, isDefault bool, refreshToken string) AccountWithToken {
	return AccountWithToken{
		UserID:       userID,
		Username:     username,
		Default:      isDefault,
		refreshToken: refreshToken,
	}
}

// RefreshToken 返回仅供显式 auth export 使用的 refresh token。
func (a AccountWithToken) RefreshToken() string { return a.refreshToken }

// String 只输出非 secret 摘要。
func (a AccountWithToken) String() string {
	return fmt.Sprintf("pixiv.AccountWithToken{user_id:%d username:%q default:%t}", a.UserID, a.Username, a.Default)
}

// GoString 返回 %#v 使用的安全摘要。
func (a AccountWithToken) GoString() string { return a.String() }

// Format 覆盖所有 fmt 格式化路径，避免调试输出暴露非导出 token 字段。
func (a AccountWithToken) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, a.String())
}

// MarshalJSON 保留非 secret 摘要的 JSON 形状，避免 token 因调试或通用编码被输出。
func (a AccountWithToken) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		UserID   int64  `json:"user_id"`
		Username string `json:"username"`
		Default  bool   `json:"default"`
	}{UserID: a.UserID, Username: a.Username, Default: a.Default})
}
