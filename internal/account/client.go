package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/fanbox/report"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk"
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// OpenAccountClient 读取指定账号，持久化 OAuth 轮换后才交出旧 SDK client。
func (s *Store) OpenAccountClient(ctx context.Context, userID int64, options legacysdk.Options) (*legacysdk.Client, error) {
	session, _, err := s.openReadSession(ctx, userID, options)
	return session.Client, err
}

func (s *Store) openReadSession(ctx context.Context, userID int64, options legacysdk.Options) (ReadSession, pixiv.Credentials, error) {
	// CAS 冲突后丢弃本次 OAuth 结果，重新读取获胜者的凭据再打开；OAuth 始终在锁外。
	for {
		if err := ctx.Err(); err != nil {
			return ReadSession{}, pixiv.Credentials{}, err
		}
		a, err := s.GetPixiv(ctx, userID)
		if err != nil {
			return ReadSession{}, pixiv.Credentials{}, fmt.Errorf("select pixiv account: %w", err)
		}
		client, credentials, err := legacysdk.OpenWith(ctx, a.Credential.Value(), options)
		if err != nil {
			return ReadSession{}, pixiv.Credentials{}, err
		}
		if err := verifyAccountIdentity(userID, credentials.UserID); err != nil {
			client.CloseIdleConnections()
			return ReadSession{}, pixiv.Credentials{}, err
		}
		if err := s.RotatePixivCredentials(ctx, a, []byte(credentials.RefreshToken())); err != nil {
			client.CloseIdleConnections()
			if errors.Is(err, ErrCredentialConflict) {
				continue
			}
			return ReadSession{}, pixiv.Credentials{}, fmt.Errorf("persist rotated pixiv credentials: %w", err)
		}
		current := pixiv.Credentials{UserID: credentials.UserID, DisplayName: credentials.Username, Revision: a.CredentialRevision + 1, ExpiresAt: credentials.ExpiresAt}
		current.SetTokens(credentials.AccessToken(), credentials.RefreshToken())
		return ReadSession{Client: client, Revision: current.Revision, createdAt: a.CreatedAt, Expires: current.ExpiresAt}, current, nil
	}
}

func verifyAccountIdentity(selected, authenticated int64) error {
	if selected <= 0 || selected != authenticated {
		return sdk.NewError("pixiv", "OpenAccountClient", sdk.LocalStateError, sdk.WithDetail("credential identity does not match selected account"))
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
		return Pixiv{}, fmt.Errorf("save pixiv account: %w", err)
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
		return nil, fmt.Errorf("select fanbox account: %w", err)
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
		return Fanbox{}, report.Wrap(err)
	}
	a := Fanbox{UserID: user.UserID, DisplayName: user.DisplayName, CreatorID: user.CreatorID, Credential: credential.New([]byte(session)), ValidatedAt: time.Now().UTC()}
	if err := s.SaveFanboxCredential(ctx, a); err != nil {
		return Fanbox{}, fmt.Errorf("save fanbox account: %w", err)
	}
	return s.GetFanbox(ctx, a.UserID)
}
