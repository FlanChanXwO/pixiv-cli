package fanbox

import "context"

// FanboxAccountRecord 是 FANBOX 持久化端口使用的非协议 DTO。
type FanboxAccountRecord struct {
	UserID             int64
	SortOrder          int64
	DisplayName        string
	CreatorID          string
	SessionID          []byte
	CredentialRevision int64
	ValidatedAt        int64
	CreatedAt          int64
	UpdatedAt          int64
}

// FanboxRepository 是 FANBOX 用例依赖的持久化端口。
type FanboxRepository interface {
	SaveFanboxCredential(context.Context, FanboxAccountRecord) error
	RotateFanboxSession(context.Context, int64, int64, []byte, int64) error
	ListFanbox(context.Context) ([]FanboxAccountRecord, error)
	GetFanbox(context.Context, int64) (FanboxAccountRecord, error)
	RemoveFanbox(context.Context, int64) error
}

// DefaultStore 是 FANBOX 显式账号选择配置的端口。
type DefaultStore interface {
	ReadFanboxDefaultUserID() (int64, bool, error)
	SetFanboxDefaultUserID(int64) error
	ClearFanboxDefaultUserID() error
}
