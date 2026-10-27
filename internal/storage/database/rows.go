package database

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
)

const (
	tablePixivAccount  = "pixiv_account"
	tableFanboxAccount = "fanbox_account"

	colUserID             = "user_id"
	colSortOrder          = "sort_order"
	colUsername           = "username"
	colRefreshToken       = "refresh_token"
	colCredentialRevision = "credential_revision"
	colPremiumStatus      = "premium_status"
	colPremiumCheckedAt   = "premium_checked_at"
	colPoolFrozenUntil    = "pool_frozen_until"
	colPoolLastSelected   = "pool_last_selected"
	colSchedulable        = "schedulable"
	colCreatedAt          = "created_at"
	colUpdatedAt          = "updated_at"
	colDisplayName        = "display_name"
	colCreatorID          = "creator_id"
	colSessionID          = "session_id"
	colValidatedAt        = "validated_at"
)

// pixivAccountRow 对应 pixiv_account 的一行；所有列显式映射。
type pixivAccountRow struct {
	UserID             int64  `gorm:"column:user_id;primaryKey;autoIncrement:false"`
	SortOrder          int64  `gorm:"column:sort_order"`
	Username           string `gorm:"column:username"`
	RefreshToken       []byte `gorm:"column:refresh_token"`
	CredentialRevision int64  `gorm:"column:credential_revision"`
	PremiumStatus      *int64 `gorm:"column:premium_status"`
	PremiumCheckedAt   *int64 `gorm:"column:premium_checked_at"`
	PoolFrozenUntil    *int64 `gorm:"column:pool_frozen_until"`
	PoolLastSelected   bool   `gorm:"column:pool_last_selected"`
	CreatedAt          int64  `gorm:"column:created_at;autoCreateTime:false"`
	UpdatedAt          int64  `gorm:"column:updated_at;autoUpdateTime:false"`
	Schedulable        bool   `gorm:"column:schedulable"`
}

func (pixivAccountRow) TableName() string { return tablePixivAccount }

func (r pixivAccountRow) account() pixivaccount.Account {
	account := pixivaccount.New(r.UserID, r.Username, r.RefreshToken)
	account.SortOrder = r.SortOrder
	account.CredentialRevision = r.CredentialRevision
	account.PremiumStatus = intPtrToBoolPtr(r.PremiumStatus)
	account.PremiumCheckedAt = r.PremiumCheckedAt
	account.PoolFrozenUntil = r.PoolFrozenUntil
	account.PoolLastSelected = r.PoolLastSelected
	account.CreatedAt = r.CreatedAt
	account.UpdatedAt = r.UpdatedAt
	account.Schedulable = r.Schedulable
	return account
}

type fanboxAccountRow struct {
	UserID             int64  `gorm:"column:user_id;primaryKey;autoIncrement:false"`
	SortOrder          int64  `gorm:"column:sort_order"`
	DisplayName        string `gorm:"column:display_name"`
	CreatorID          string `gorm:"column:creator_id"`
	SessionID          []byte `gorm:"column:session_id"`
	CredentialRevision int64  `gorm:"column:credential_revision"`
	ValidatedAt        int64  `gorm:"column:validated_at"`
	CreatedAt          int64  `gorm:"column:created_at;autoCreateTime:false"`
	UpdatedAt          int64  `gorm:"column:updated_at;autoUpdateTime:false"`
}

func (fanboxAccountRow) TableName() string { return tableFanboxAccount }

func (r fanboxAccountRow) account() fanboxaccount.Account {
	account := fanboxaccount.New(r.UserID, r.DisplayName, r.CreatorID, r.SessionID)
	account.SortOrder = r.SortOrder
	account.CredentialRevision = r.CredentialRevision
	account.ValidatedAt = r.ValidatedAt
	account.CreatedAt = r.CreatedAt
	account.UpdatedAt = r.UpdatedAt
	return account
}

type poolCandidateRow struct {
	UserID           int64  `gorm:"column:user_id"`
	SortOrder        int64  `gorm:"column:sort_order"`
	Schedulable      bool   `gorm:"column:schedulable"`
	PoolFrozenUntil  *int64 `gorm:"column:pool_frozen_until"`
	PoolLastSelected bool   `gorm:"column:pool_last_selected"`
}

func (r poolCandidateRow) candidate(now int64) pixivaccount.PoolCandidate {
	return pixivaccount.PoolCandidate{
		UserID:           r.UserID,
		SortOrder:        r.SortOrder,
		Schedulable:      r.Schedulable,
		PoolFrozenUntil:  r.PoolFrozenUntil,
		PoolLastSelected: r.PoolLastSelected,
		Eligible:         r.Schedulable && (r.PoolFrozenUntil == nil || *r.PoolFrozenUntil <= now),
	}
}
