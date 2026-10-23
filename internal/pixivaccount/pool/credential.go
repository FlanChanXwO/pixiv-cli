package pool

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// CredentialStore 是 rotation 所需的最窄 account leaf port。
type CredentialStore interface {
	RotatePixivCredentials(context.Context, int64, int64, []byte) error
}

// RotateCredential 在 Pixiv 产品边界执行身份检查和 revision CAS。
// pool policy 决定何时轮换；本函数只负责将已验证的凭据交给 account
// repository 持久化。
func RotateCredential(ctx context.Context, repository CredentialStore, selectedUserID, authenticatedUserID, revision int64, refreshToken []byte) error {
	if err := VerifyAccountIdentity(selectedUserID, authenticatedUserID); err != nil {
		return err
	}
	if repository == nil {
		return errors.New("pixiv credential repository is not configured")
	}
	return repository.RotatePixivCredentials(ctx, selectedUserID, revision, refreshToken)
}

// VerifyAccountIdentity rejects a credential whose authenticated UID does not
// match the selected local account before rotation or content requests.
func VerifyAccountIdentity(selectedUserID, authenticatedUserID int64) error {
	if selectedUserID <= 0 || authenticatedUserID <= 0 || selectedUserID != authenticatedUserID {
		return sdk.NewError("pixiv", "OpenAccountClient", sdk.LocalStateError, sdk.WithDetail("credential identity does not match selected account"))
	}
	return nil
}
