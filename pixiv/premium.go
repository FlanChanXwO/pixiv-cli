package pixiv

import (
	"context"
	"errors"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
)

// PremiumStatus 返回当前认证账号的会员资格，并在已保存账号上复用未过期缓存。
func (c *Client) PremiumStatus(ctx context.Context) (*PremiumStatus, error) {
	return c.premiumStatus(ctx, false)
}

// RefreshPremiumStatus 忽略已有缓存，重新读取当前账号 profile 并写回本地账号状态。
// 调用方可在 auth refresh 这类显式维护操作中使用它。
func (c *Client) RefreshPremiumStatus(ctx context.Context) (*PremiumStatus, error) {
	return c.premiumStatus(ctx, true)
}

func (c *Client) premiumStatus(ctx context.Context, force bool) (result *PremiumStatus, err error) {
	if scoped, snapshotErr := c.operationClient(ctx, OperationPremiumStatus); snapshotErr != nil {
		return nil, snapshotErr
	} else if scoped != c {
		return scoped.premiumStatus(ctx, force)
	}
	if !c.authenticated || c.authenticatedUserID <= 0 {
		return nil, localRouteError(CodeUnsupported, OperationPremiumStatus, 0, 0, errors.New("authenticated account identity is required to query Pixiv Premium status"))
	}
	now := time.Now().UTC()
	if !force {
		if cached, ok := c.premiumCache(now); ok {
			return cached, nil
		}
	}
	detail, err := c.UserDetail(ctx, UserDetailRequest{UserID: c.authenticatedUserID})
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, newUserError(CodeMalformedUpstreamResponse, OperationPremiumStatus, BackendAppAPI, false, 0, c.authenticatedUserID, errors.New("Pixiv user detail is empty"))
	}
	result = &PremiumStatus{IsPremium: detail.Profile.IsPremium, CheckedAt: now}
	if err := c.storePremiumStatus(*result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) premiumCache(now time.Time) (*PremiumStatus, bool) {
	c.premiumStatusMu.Lock()
	defer c.premiumStatusMu.Unlock()
	if c.cachedPremiumStatus == nil || c.premiumStatusCheckedAt.IsZero() || c.premiumStatusCacheTTL <= 0 || c.premiumStatusCheckedAt.After(now) || now.Sub(c.premiumStatusCheckedAt) > c.premiumStatusCacheTTL {
		return nil, false
	}
	return &PremiumStatus{IsPremium: *c.cachedPremiumStatus, CheckedAt: c.premiumStatusCheckedAt}, true
}

func (c *Client) storePremiumStatus(status PremiumStatus) error {
	c.premiumStatusMu.Lock()
	premium := status.IsPremium
	c.cachedPremiumStatus = &premium
	c.premiumStatusCheckedAt = status.CheckedAt
	c.premiumStatusMu.Unlock()

	if c.premiumStatusAuthPath == "" {
		return nil
	}
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()
	store, err := auth.LoadAuthStore(c.premiumStatusAuthPath)
	if err != nil {
		return localSnapshotError(OperationPremiumStatus, markLocalState(localStateStageAuth, err))
	}
	index, account, ok := store.Get(c.authenticatedUserID)
	if !ok {
		return newUserError(CodeInvalidArgument, OperationPremiumStatus, "", false, 0, c.authenticatedUserID, errors.New("selected account does not exist"))
	}
	accountPremium := status.IsPremium
	account.PremiumStatus = &accountPremium
	checkedAt := status.CheckedAt
	account.PremiumStatusCheckedAt = &checkedAt
	store.Accounts[index] = account
	if err := auth.SaveAuthStore(c.premiumStatusAuthPath, store); err != nil {
		return localSnapshotError(OperationPremiumStatus, markLocalState(localStateStageAuth, err))
	}
	return nil
}
