// Package accounts 管理 MCP 进程共享的 Pixiv 账号选择：列出本地账号、报告选中状态，
// 并在登录成功后接管新账号。
//
// 包的组成：[Selection] 是读本地账号快照并读写共享选择的入口，[Status] 与 [Account]
// 是公开的状态值，[Login] 与 [NewLogin] 负责远程登录接力。这里只保存账号 ID 和本地
// 摘要，Pixiv 凭据始终留在账号存储中。
package accounts

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
)

var (
	// ErrInvalidUserID 表示显式传入的 user_id 小于等于零。
	ErrInvalidUserID = errors.New("user_id must be positive")
	// ErrAccountNotFound 表示本地账号列表里没有请求的账号。
	ErrAccountNotFound = errors.New("account_not_found")
)

// Account 保存本地账号摘要。HasCredentials 只说明本地存有凭据，凭据是否仍被上游接受
// 要由 SDK 实际请求确认。
type Account struct {
	UserID         int64  `json:"user_id"`
	Username       string `json:"username"`
	HasCredentials bool   `json:"has_credentials"`
}

// LocalSnapshot 区分 CLI 显式设置的默认账号（DefaultUserID 非零）与仅按列表顺序给出的
// 候选。
type LocalSnapshot struct {
	Accounts      []Account
	DefaultUserID int64
}

const (
	// SelectionUnknown 表示还没读取到账号状态。
	SelectionUnknown = "unknown"
	// SelectionRequired 表示需要显式选择一个账号。
	SelectionRequired = "selection_required"
	// SelectionNoLocal 表示本地没有任何账号。
	SelectionNoLocal = "no_local_account"
	// SelectionNotFound 表示已选账号不在本地列表中。
	SelectionNotFound = "account_not_found"
	// SelectionSelected 表示已选账号存在且带有本地凭据。
	SelectionSelected = "selected"
	// SelectionNoCredentials 表示已选账号存在但缺少本地凭据。
	SelectionNoCredentials = "credentials_missing"
)

const (
	// CredentialUnknown 表示还没读取到凭据状态。
	CredentialUnknown = "unknown"
	// CredentialMissing 表示账号没有本地凭据。
	CredentialMissing = "missing"
	// CredentialPresentUnverified 表示本地存有凭据，但尚未由上游确认有效。
	CredentialPresentUnverified = "present_unverified"
)

// Status 报告本地选择与凭据存在情况；上游会话是否仍然有效由 SDK 请求决定。
type Status struct {
	SelectedUserID  int64     `json:"selected_user_id"`
	SelectionState  string    `json:"selection_state"`
	CredentialState string    `json:"credential_state"`
	Accounts        []Account `json:"accounts"`
}

// Selection 在读本地账号快照的同时维护所有连接器共享的 MCP 账号选择；Load 只读本地
// 数据。
type Selection struct {
	Store auth.Store
	Load  func(context.Context) (LocalSnapshot, error)
}

// Status 在尚未选择时采用一次初始账号（CLI 默认，或唯一的本地账号），之后只报告已保存
// 的身份，不会因该账号消失而自动换号。
func (m Selection) Status(ctx context.Context) (Status, error) {
	if m.Load == nil {
		return Status{}, errors.New("MCP account reader is not configured")
	}
	local, err := m.Load(ctx)
	if err != nil {
		return Status{}, err
	}
	state, err := m.Store.Read(ctx)
	if err != nil {
		return Status{}, err
	}
	selected := state.SelectedPixivUserID
	if selected == 0 {
		candidate := local.DefaultUserID
		if candidate == 0 && len(local.Accounts) == 1 {
			candidate = local.Accounts[0].UserID
		}
		if candidate > 0 {
			selected, err = m.Store.InitializePixivUser(ctx, candidate)
			if err != nil {
				return Status{}, err
			}
		}
	}
	return statusFor(local.Accounts, selected), nil
}

func statusFor(local []Account, selected int64) Status {
	if local == nil {
		local = []Account{}
	}
	result := Status{SelectedUserID: selected, SelectionState: SelectionRequired, CredentialState: CredentialUnknown, Accounts: local}
	if selected == 0 {
		if len(local) == 0 {
			result.SelectionState = SelectionNoLocal
		}
		return result
	}
	result.SelectionState = SelectionNotFound
	result.CredentialState = CredentialMissing
	for _, account := range local {
		if account.UserID != selected {
			continue
		}
		if account.HasCredentials {
			result.SelectionState = SelectionSelected
			result.CredentialState = CredentialPresentUnverified
		} else {
			result.SelectionState = SelectionNoCredentials
		}
		break
	}
	return result
}

// Use 先确认账号存在于本地快照，再提交共享选择；CLI 的默认账号不受影响。
func (m Selection) Use(ctx context.Context, userID int64) (Status, error) {
	if userID <= 0 {
		return Status{}, ErrInvalidUserID
	}
	if m.Load == nil {
		return Status{}, errors.New("MCP account reader is not configured")
	}
	local, err := m.Load(ctx)
	if err != nil {
		return Status{}, err
	}
	for _, account := range local.Accounts {
		if account.UserID != userID {
			continue
		}
		if err := m.Store.SelectPixivUser(ctx, userID); err != nil {
			return Status{}, err
		}
		return statusFor(local.Accounts, userID), nil
	}
	return Status{}, ErrAccountNotFound
}

// Resolve 要求当前选择在本地已有凭据，返回账号 ID；凭据的上游有效性由后续 SDK 请求
// 判断。
func (m Selection) Resolve(ctx context.Context) (int64, error) {
	status, err := m.Status(ctx)
	if err != nil {
		return 0, err
	}
	if status.SelectionState != SelectionSelected {
		return 0, errors.New(status.SelectionState)
	}
	return status.SelectedUserID, nil
}

// SelectAfterLogin 只在当前选择没有本地凭据时接管刚登录的账号。查找与条件提交在同一把
// 状态文件锁内完成，避免用过期可用性覆盖用户已做的显式选择。
func (m Selection) SelectAfterLogin(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	if m.Load == nil {
		return false, errors.New("MCP account reader is not configured")
	}
	return m.Store.SelectPixivUserIf(ctx, userID, func(selected int64) (bool, error) {
		local, err := m.Load(ctx)
		if err != nil {
			return false, err
		}
		if statusFor(local.Accounts, selected).SelectionState == "selected" {
			return false, nil
		}
		if statusFor(local.Accounts, userID).SelectionState != "selected" {
			return false, ErrAccountNotFound
		}
		return true, nil
	})
}
