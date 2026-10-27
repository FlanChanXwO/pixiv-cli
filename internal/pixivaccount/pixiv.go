// Package pixivaccount 提供 Pixiv 本地账号领域、凭据轮换持久化与公开 SDK 会话。
//
// 账号凭据由本包保持 opaque：调用方只能取得副本，格式化与日志不会输出 refresh token。
//
// 包的组成：[Service] 负责账号选择、登录与凭据轮换，[Account] 是保存的账号记录，
// [Repository] 与 [DefaultStore] 是存储端口，[ReadSessions] 为 MCP 复用读取会话，
// [Chooser] 与 [Scheduler] 提供账号池的选择策略与执行编排。
package pixivaccount

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
)

// Account 描述本机保存的一个 Pixiv 账号记录；除 refresh token 外的字段是会话与存储共用
// 的非 secret 状态。
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

	Credential credential.Secret `json:"-"`
}

// New 会立即复制 refreshToken，调用方之后修改原切片不影响账号。
func New(userID int64, username string, refreshToken []byte) Account {
	return Account{UserID: userID, Username: username, Credential: credential.New(refreshToken)}
}

// RefreshTokenCopy 返回 refresh token 的独立副本。
func (a Account) RefreshTokenCopy() []byte { return a.Credential.Copy() }

// HasRefreshToken 报告账号是否保存了非空凭据。
func (a Account) HasRefreshToken() bool { return !a.Credential.Empty() }

// PoolCandidate 描述 DB 事务交给纯 chooser 的 non-secret 快照项。
type PoolCandidate struct {
	UserID           int64
	SortOrder        int64
	Schedulable      bool
	PoolFrozenUntil  *int64
	PoolLastSelected bool
	Eligible         bool
}

// PoolSnapshot 描述一次选择事务中观察到的候选集合和 marker。
type PoolSnapshot struct {
	Candidates          []PoolCandidate
	MarkerUserID        *int64
	MarkerSortOrder     *int64
	EarliestFrozenUntil *int64
}

// Chooser 只基于事务快照选择 UID；实现不得执行 IO 或修改持久状态。
type Chooser func(PoolSnapshot) (int64, error)

// PoolStatus 表示一个时间点上的完整 non-secret 调度快照。
type PoolStatus struct {
	Accounts            []PoolCandidate
	EarliestFrozenUntil *int64
}

// PoolSelectionKind 区分账号池没有候选的真实原因。
type PoolSelectionKind string

// PoolSelectionNoLocalAccount、PoolSelectionNoSchedulable、PoolSelectionAllFrozen 与
// PoolSelectionExhausted 区分为何没有可用账号，供调用方映射稳定错误。
const (
	PoolSelectionNoLocalAccount PoolSelectionKind = "no_local_account"
	PoolSelectionNoSchedulable  PoolSelectionKind = "no_schedulable_account"
	PoolSelectionAllFrozen      PoolSelectionKind = "all_frozen"
	PoolSelectionExhausted      PoolSelectionKind = "exhausted"
)

// PoolSelectionError 描述不含 credential 的选择结果。
type PoolSelectionError struct {
	Kind                PoolSelectionKind
	EarliestFrozenUntil *int64
}

// Error 输出固定英文前缀并拼接 Kind 文本。Kind 是导出字段，调用方可写入任意值，
// 因此输出不保证不含凭据或候选列表；nil 接收者也能安全调用。
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

// Repository 定义 database 直接实现的 Pixiv account storage port。
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

// DefaultStore 定义当前账号选择配置的 storage port；它不保存 credential。
type DefaultStore interface {
	ReadPixivDefaultUserID() (int64, bool, error)
	SetPixivDefaultUserID(int64) error
	ClearPixivDefaultUserID() error
}
