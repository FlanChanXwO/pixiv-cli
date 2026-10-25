// Package fanboxaccount 定义 FANBOX 本地账号领域和独立 storage port。
// FANBOX session 不与 Pixiv refresh token 共享类型或生命周期。
package fanboxaccount

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
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

	Credential credential.Secret `json:"-"`
}

// New 创建一个带 opaque FANBOX session 的账号值，并复制输入凭据。
func New(userID int64, displayName, creatorID string, sessionID []byte) Account {
	return Account{UserID: userID, DisplayName: displayName, CreatorID: creatorID, Credential: credential.New(sessionID)}
}

// SessionIDCopy 返回 session 的 defensive copy。
func (a Account) SessionIDCopy() []byte { return a.Credential.Copy() }

// HasSession reports whether the account contains a non-empty session.
func (a Account) HasSession() bool { return !a.Credential.Empty() }

// ErrNotFound 表示按 ID 找不到账号。
var ErrNotFound = errors.New("fanbox account not found")

// Repository 是 database 直接实现的 FANBOX account storage port。
type Repository interface {
	SaveFanboxCredential(context.Context, Account) error
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
