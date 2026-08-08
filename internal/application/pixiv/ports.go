package pixiv

import (
	"context"
)

// PixivAccountRecord 是 Pixiv 持久化端口使用的非协议 DTO。
// 它只在 application/pixiv 与 bootstrap 的 persistence adapter 之间流动，
// 不把 SQLite 记录类型泄漏到用例层。
type PixivAccountRecord struct {
	UserID             int64
	SortOrder          int64
	Username           string
	RefreshToken       []byte
	CredentialRevision int64
	PremiumStatus      *bool
	PremiumCheckedAt   *int64
	PoolFrozenUntil    *int64
	PoolLastSelected   bool
	CreatedAt          int64
	UpdatedAt          int64
	Schedulable        bool
}

// PixivPoolAccountRecord 是账号池状态的脱敏持久化 DTO。
type PixivPoolAccountRecord struct {
	UserID           int64
	SortOrder        int64
	Schedulable      bool
	PoolFrozenUntil  *int64
	PoolLastSelected bool
	Eligible         bool
}

// PixivPoolStatusRecord 是一个时间点上的账号池快照。
type PixivPoolStatusRecord struct {
	Accounts            []PixivPoolAccountRecord
	EarliestFrozenUntil *int64
}

// PoolStrategy 是 repository 端口接受的已验证选择策略。
type PoolStrategy string

const (
	PoolStrategyRoundRobin PoolStrategy = "round_robin"
	PoolStrategyRandom     PoolStrategy = "random"
)

// PoolSelectionError 由持久化 adapter 把本地账号池无候选原因转换为应用层
// 可识别的脱敏错误；应用层无需依赖 SQLite 包的错误类型。
type PoolSelectionError struct {
	Kind                string
	EarliestFrozenUntil *int64
}

const (
	PoolSelectionNoLocalAccount = "no_local_account"
	PoolSelectionNoSchedulable  = "no_schedulable_account"
	PoolSelectionAllFrozen      = "all_frozen"
	PoolSelectionExhausted      = "exhausted"
)

func (e *PoolSelectionError) Error() string {
	if e == nil {
		return "pixiv account pool selection failed"
	}
	return "pixiv account pool selection failed: " + e.Kind
}

// PixivRepository 是 Pixiv 用例依赖的持久化端口。实现位于 bootstrap 下的
// adapter，避免 application 直接引用 persistence/authdb。
type PixivRepository interface {
	SavePixivCredential(context.Context, PixivAccountRecord) error
	SavePixivCredentials(context.Context, []PixivAccountRecord) error
	UpdatePixivMetadata(context.Context, int64, string, *bool, *int64) error
	RotatePixivCredentials(context.Context, int64, int64, []byte) error
	ListPixiv(context.Context) ([]PixivAccountRecord, error)
	GetPixiv(context.Context, int64) (PixivAccountRecord, error)
	RemovePixiv(context.Context, int64) error
	SetPixivSchedulable(context.Context, []int64, bool) error
	SetAllPixivSchedulable(context.Context, bool) error
	ListPixivPoolStatus(context.Context, int64) (PixivPoolStatusRecord, error)
	SelectPooledPixiv(context.Context, int64, PoolStrategy, []int64) (PixivAccountRecord, error)
	FreezePooledPixiv(context.Context, int64, int64) error
}

// DefaultStore 是显式账号选择配置的端口。它不包含 token，也不负责账号
// 列表；配置文件的读写实现由 bootstrap 注入。
type DefaultStore interface {
	ReadPixivDefaultUserID() (int64, bool, error)
	SetPixivDefaultUserID(int64) error
	ClearPixivDefaultUserID() error
}
