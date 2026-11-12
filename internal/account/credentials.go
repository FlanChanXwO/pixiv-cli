package account

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type savedCredentials struct {
	store     *Store
	userID    int64
	createdAt time.Time
}

func (s savedCredentials) Load(ctx context.Context) (pixiv.Credentials, error) {
	account, err := s.store.GetPixiv(ctx, s.userID)
	if err != nil {
		return pixiv.Credentials{}, err
	}
	if !account.CreatedAt.Equal(s.createdAt) {
		return pixiv.Credentials{}, ErrCredentialConflict
	}
	return pixiv.Credentials{UserID: account.UserID, DisplayName: account.Username,
		Revision: account.CredentialRevision, RefreshToken: account.Credential}, nil
}

func (s savedCredentials) CompareAndSwap(ctx context.Context, expected int64, replacement pixiv.Credentials) (bool, error) {
	if err := verifyAccountIdentity(s.userID, replacement.UserID); err != nil {
		return false, err
	}
	err := s.store.RotatePixivCredentials(ctx, Pixiv{UserID: s.userID, CredentialRevision: expected, CreatedAt: s.createdAt}, replacement.RefreshToken.Copy())
	if errors.Is(err, ErrCredentialConflict) {
		return false, nil
	}
	return err == nil, err
}

// OpenSDKClient 绑定指定账号的最新凭据；SDK 刷新时先提交 revision CAS 再发内容请求。
// OAuth 在仓库方法返回后执行，网络请求期间不持有存储锁。
func (s *Store) OpenSDKClient(ctx context.Context, userID int64, options pixiv.Options) (*pixiv.Client, error) {
	account, err := s.GetPixiv(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("select pixiv account: %w", err)
	}
	return pixiv.New(savedCredentials{store: s, userID: userID, createdAt: account.CreatedAt}, options)
}
