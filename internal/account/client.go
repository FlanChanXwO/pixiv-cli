package account

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/report"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func verifyAccountIdentity(selected, authenticated int64) error {
	if selected <= 0 || selected != authenticated {
		return &sdk.Error{Operation: "OpenSDKClient", Reason: sdk.LocalStateError, Message: "credential identity does not match selected account"}
	}
	return nil
}

// ImportPixiv 在线验证一个显式 token，保存轮换结果；默认账号由调用方处理。
func (s *Store) ImportPixiv(ctx context.Context, refreshToken string, options pixiv.Options) (Pixiv, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	store, err := pixiv.NewMemoryCredentials(pixiv.NewCredentials(refreshToken))
	if err != nil {
		return Pixiv{}, err
	}
	client, err := pixiv.New(store, options)
	if err != nil {
		return Pixiv{}, err
	}
	defer client.CloseIdleConnections()
	credentials, err := client.Credentials(ctx)
	if err != nil {
		return Pixiv{}, err
	}
	return s.savePixivLogin(ctx, credentials)
}

func (s *Store) savePixivLogin(ctx context.Context, credentials pixiv.Credentials) (Pixiv, error) {
	a := Pixiv{UserID: credentials.UserID, Username: credentials.DisplayName, Credential: credentials.RefreshToken}
	if err := s.SavePixivCredential(ctx, a); err != nil {
		return Pixiv{}, i18n.Errorf("error.save_pixiv_account", err)
	}
	saved, err := s.GetPixiv(ctx, a.UserID)
	if err != nil {
		return a, err
	}
	return saved, nil
}

// OpenFanboxClient 打开指定本地账号的 SDK client，不读取默认账号或网络配置。
func (s *Store) OpenFanboxClient(ctx context.Context, userID int64, options fanbox.Options) (*fanbox.Client, error) {
	a, err := s.GetFanbox(ctx, userID)
	if err != nil {
		return nil, i18n.Errorf("error.select_fanbox_account", err)
	}
	return fanbox.New(a.Credential.Value(), options)
}

// ImportFanbox 在线校验显式 session 并保存；Pixiv 凭据始终独立。
func (s *Store) ImportFanbox(ctx context.Context, session string, options fanbox.Options) (result Fanbox, err error) {
	client, err := fanbox.New(session, options)
	if err != nil {
		return Fanbox{}, err
	}
	defer func() { err = errors.Join(err, client.Close(context.WithoutCancel(ctx))) }()
	user, err := client.CurrentUser(ctx)
	if err != nil {
		return Fanbox{}, report.Wrap(err, "CurrentUser")
	}
	a := Fanbox{UserID: user.UserID, DisplayName: user.DisplayName, CreatorID: user.CreatorID, Credential: credential.New([]byte(session)), ValidatedAt: time.Now().UTC()}
	if err := s.SaveFanboxCredential(ctx, a); err != nil {
		return Fanbox{}, i18n.Errorf("error.save_fanbox_account", err)
	}
	return s.GetFanbox(ctx, a.UserID)
}
