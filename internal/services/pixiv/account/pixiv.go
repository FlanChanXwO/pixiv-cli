// Package pixiv 定义 Pixiv 本地账号领域和持久化端口。
//
// 账号凭据由本包保持 opaque：调用方只能取得 defensive copy，默认格式化也
// 不会把 refresh token 写入日志、错误或测试输出。
package pixiv

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Account 是一个已保存的 Pixiv 账号记录。除 refresh token 外的字段是
// account/session 与 storage 共同需要的非 secret 状态。
type Account struct {
	UserID             int64
	SortOrder          int64
	Username           string
	CredentialRevision int64
	PremiumStatus      *bool
	PremiumCheckedAt   *int64
	PoolFrozenUntil    *int64
	PoolLastSelected   bool
	CreatedAt          int64
	UpdatedAt          int64
	Schedulable        bool

	refreshToken []byte
}

// New 创建一个带 opaque refresh token 的账号值。输入凭据会立即复制。
func New(userID int64, username string, refreshToken []byte) Account {
	return Account{UserID: userID, Username: username, refreshToken: cloneBytes(refreshToken)}
}

// RefreshTokenCopy 返回 refresh token 的 defensive copy。
func (a Account) RefreshTokenCopy() []byte { return cloneBytes(a.refreshToken) }

// HasRefreshToken reports whether the account contains a non-empty credential.
func (a Account) HasRefreshToken() bool { return len(a.refreshToken) != 0 }

// String 只输出安全摘要；refresh token 永远不进入默认格式化结果。
func (a Account) String() string {
	return fmt.Sprintf("pixiv.Account{user_id:%d username:%q credential_revision:%d}", a.UserID, a.Username, a.CredentialRevision)
}

// GoString 返回 %#v 使用的安全摘要，避免 fmt 回退到结构体默认格式并暴露凭据字段。
func (a Account) GoString() string { return a.String() }

// Format 覆盖所有 fmt 格式化路径，避免带 %#v 的调试输出回退到结构体字段。
func (a Account) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, a.String())
}

// PoolCandidate 是 DB 事务交给纯 chooser 的 non-secret 快照项。
type PoolCandidate struct {
	UserID           int64
	SortOrder        int64
	Schedulable      bool
	PoolFrozenUntil  *int64
	PoolLastSelected bool
	Eligible         bool
}

// PoolSnapshot 是一次选择事务中观察到的候选集合和 marker。
type PoolSnapshot struct {
	Candidates          []PoolCandidate
	MarkerUserID        *int64
	MarkerSortOrder     *int64
	EarliestFrozenUntil *int64
}

// Chooser 只基于事务快照选择 UID；实现不得执行 IO 或修改持久状态。
type Chooser func(PoolSnapshot) (int64, error)

// PoolStatus 是一个时间点上的完整 non-secret 调度快照。
type PoolStatus struct {
	Accounts            []PoolCandidate
	EarliestFrozenUntil *int64
}

// PoolSelectionKind 区分账号池没有候选的真实原因。
type PoolSelectionKind string

const (
	PoolSelectionNoLocalAccount PoolSelectionKind = "no_local_account"
	PoolSelectionNoSchedulable  PoolSelectionKind = "no_schedulable_account"
	PoolSelectionAllFrozen      PoolSelectionKind = "all_frozen"
	PoolSelectionExhausted      PoolSelectionKind = "exhausted"
)

// PoolSelectionError 是不含 credential 的选择结果。
type PoolSelectionError struct {
	Kind                PoolSelectionKind
	EarliestFrozenUntil *int64
}

func (e *PoolSelectionError) Error() string {
	if e == nil {
		return "pixiv account pool selection failed"
	}
	return "pixiv account pool selection failed: " + string(e.Kind)
}

// ErrNotFound 表示按 ID 找不到账号。
var ErrNotFound = errors.New("pixiv account not found")

// ErrCredentialConflict 表示 rotation 使用了过期的 credential revision。
var ErrCredentialConflict = errors.New("pixiv account credential revision conflict")

// Repository 是 database 直接实现的 Pixiv account storage port。
type Repository interface {
	SavePixivCredential(context.Context, Account) error
	SavePixivCredentials(context.Context, []Account) error
	UpdatePixivMetadata(context.Context, int64, string, *bool, *int64) error
	RotatePixivCredentials(context.Context, int64, int64, []byte) error
	ListPixiv(context.Context) ([]Account, error)
	GetPixiv(context.Context, int64) (Account, error)
	RemovePixiv(context.Context, int64) error
	SetPixivSchedulable(context.Context, []int64, bool) error
	SetAllPixivSchedulable(context.Context, bool) error
	ListPixivPoolStatus(context.Context, int64) (PoolStatus, error)
	SelectPixiv(context.Context, int64, []int64, Chooser) (Account, error)
	Freeze(context.Context, int64, int64) error
}

// DefaultStore 是当前账号选择配置的 storage port；它不保存 credential。
type DefaultStore interface {
	ReadPixivDefaultUserID() (int64, bool, error)
	SetPixivDefaultUserID(int64) error
	ClearPixivDefaultUserID() error
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}
