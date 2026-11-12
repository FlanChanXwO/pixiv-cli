package account

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
)

// Pixiv 保存本地 Pixiv 账号与凭据版本；Credential 只经显式认证或文件编码读取。
// 时间均为 UTC；PremiumStatus 的 nil 表示尚未检查。
type Pixiv struct {
	UserID             int64             `json:"user_id"`
	Username           string            `json:"username"`
	CredentialRevision int64             `json:"revision"`
	PremiumStatus      *bool             `json:"premium"`
	PremiumCheckedAt   *time.Time        `json:"premium_checked_at"`
	PoolLastSelected   bool              `json:"-"` // 从文件顶层的上次选择 UID 推导，不重复保存。
	Pool               Membership        `json:"pool"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	Credential         credential.Secret `json:"-"`
}

// Membership 保存是否参与调度与限流冻结的截止时间；nil 表示未冻结。
type Membership struct {
	Enabled     bool       `json:"enabled"`
	FrozenUntil *time.Time `json:"frozen_until"`
}

// UnmarshalJSON 要求 enabled 显式存在，以免损坏的文件静默改变账号池成员身份。
func (m *Membership) UnmarshalJSON(body []byte) error {
	var value struct {
		Enabled     *bool      `json:"enabled"`
		FrozenUntil *time.Time `json:"frozen_until"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if value.Enabled == nil {
		return errors.New("account: missing pool enabled flag")
	}
	m.Enabled = *value.Enabled
	m.FrozenUntil = value.FrozenUntil
	return nil
}
