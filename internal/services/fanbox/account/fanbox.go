// Package fanbox 定义 FANBOX 本地账号领域和独立 storage port。
// FANBOX session 不与 Pixiv refresh token 共享类型或生命周期。
package fanbox

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Account 是一个已保存的 FANBOX 账号记录。session ID 是 opaque credential。
type Account struct {
	UserID             int64
	SortOrder          int64
	DisplayName        string
	CreatorID          string
	CredentialRevision int64
	ValidatedAt        int64
	CreatedAt          int64
	UpdatedAt          int64

	sessionID []byte
}

// New 创建一个带 opaque FANBOX session 的账号值，并复制输入凭据。
func New(userID int64, displayName, creatorID string, sessionID []byte) Account {
	return Account{UserID: userID, DisplayName: displayName, CreatorID: creatorID, sessionID: cloneBytes(sessionID)}
}

// SessionIDCopy 返回 session 的 defensive copy。
func (a Account) SessionIDCopy() []byte { return cloneBytes(a.sessionID) }

// HasSession reports whether the account contains a non-empty session.
func (a Account) HasSession() bool { return len(a.sessionID) != 0 }

// String 只输出安全摘要，不包含 FANBOXSESSID。
func (a Account) String() string {
	return fmt.Sprintf("fanbox.Account{user_id:%d display_name:%q credential_revision:%d}", a.UserID, a.DisplayName, a.CredentialRevision)
}

// GoString 返回 %#v 使用的安全摘要，避免 fmt 回退到结构体默认格式并暴露 session 字段。
func (a Account) GoString() string { return a.String() }

// Format 覆盖所有 fmt 格式化路径，避免私有 session 字段回退到结构体输出。
func (a Account) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, a.String())
}

// ErrNotFound 表示按 ID 找不到账号。
var ErrNotFound = errors.New("fanbox account not found")

// ErrCredentialConflict 表示 session rotation 使用了过期 revision。
var ErrCredentialConflict = errors.New("fanbox account credential revision conflict")

// Repository 是 database 直接实现的 FANBOX account storage port。
type Repository interface {
	SaveFanboxCredential(context.Context, Account) error
	RotateFanboxSession(context.Context, int64, int64, []byte, int64) error
	ListFanbox(context.Context) ([]Account, error)
	GetFanbox(context.Context, int64) (Account, error)
	RemoveFanbox(context.Context, int64) error
}

// DefaultStore 是 FANBOX 当前账号选择配置的 storage port。
type DefaultStore interface {
	ReadFanboxDefaultUserID() (int64, bool, error)
	SetFanboxDefaultUserID(int64) error
	ClearFanboxDefaultUserID() error
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}
