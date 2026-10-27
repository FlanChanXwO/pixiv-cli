// Package accounts owns MCP account selection, without holding product credentials.
package accounts

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
)

var (
	ErrInvalidUserID   = errors.New("user_id must be positive")
	ErrAccountNotFound = errors.New("account_not_found")
)

// Account is a local, non-secret account summary; credentials have not been checked upstream.
type Account struct {
	UserID         int64  `json:"user_id"`
	Username       string `json:"username"`
	HasCredentials bool   `json:"has_credentials"`
}

// LocalSnapshot distinguishes an explicit CLI default from implicit list ordering.
type LocalSnapshot struct {
	Accounts      []Account
	DefaultUserID int64
}

// Published local-selection states.
const (
	SelectionUnknown       = "unknown"
	SelectionRequired      = "selection_required"
	SelectionNoLocal       = "no_local_account"
	SelectionNotFound      = "account_not_found"
	SelectionSelected      = "selected"
	SelectionNoCredentials = "credentials_missing"
)

// Published credential states.
const (
	CredentialUnknown           = "unknown"
	CredentialMissing           = "missing"
	CredentialPresentUnverified = "present_unverified"
)

// Status reports local selection, not the real-time validity of an upstream session.
type Status struct {
	SelectedUserID  int64     `json:"selected_user_id"`
	SelectionState  string    `json:"selection_state"`
	CredentialState string    `json:"credential_state"`
	Accounts        []Account `json:"accounts"`
}

// Selection shares one private MCP account choice across connectors. Load must stay local.
type Selection struct {
	Store auth.Store
	Load  func(context.Context) (LocalSnapshot, error)
}

// Status adopts the initial selection once and otherwise reports the saved identity without fallback.
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

// Use checks local existence before committing the shared selection; CLI defaults are untouched.
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

// Resolve requires usable local credentials; their actual upstream validity is checked by the SDK.
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

// SelectAfterLogin adopts a saved account only when the current selection lacks
// local credentials. The state lock spans the lookup and conditional commit so
// an explicit choice cannot be overwritten using stale availability, even for the same ID.
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
