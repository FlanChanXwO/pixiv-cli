package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

type savedCredentials struct {
	repository Repository
	userID     int64
}

func (s savedCredentials) Load(ctx context.Context) (pixiv.Credentials, error) {
	account, err := s.repository.GetPixiv(ctx, s.userID)
	if err != nil {
		return pixiv.Credentials{}, err
	}
	return pixiv.Credentials{UserID: account.UserID, DisplayName: account.Username,
		Revision: account.CredentialRevision, RefreshToken: account.Credential}, nil
}

func (s savedCredentials) CompareAndSwap(ctx context.Context, expected int64, replacement pixiv.Credentials) (bool, error) {
	if err := verifyAccountIdentity(s.userID, replacement.UserID); err != nil {
		return false, err
	}
	err := s.repository.RotatePixivCredentials(ctx, s.userID, expected, replacement.RefreshToken.Copy())
	if errors.Is(err, ErrCredentialConflict) {
		return false, nil
	}
	return err == nil, err
}

// OpenSDKClient 绑定指定账号的最新凭据；SDK 刷新时先提交 revision CAS 再发内容请求。
// OAuth 在仓库方法返回后执行，网络请求期间不持有存储锁。
func (s *Service) OpenSDKClient(ctx context.Context, userID int64, options pixiv.Options) (*pixiv.Client, error) {
	if _, err := s.repo.GetPixiv(ctx, userID); err != nil {
		return nil, fmt.Errorf("select pixiv account: %w", err)
	}
	return pixiv.New(savedCredentials{repository: s.repo, userID: userID}, options)
}
