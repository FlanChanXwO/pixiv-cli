package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
)

// OpenAccountClient 读取指定账号，持久化 OAuth 轮换后才交出旧 SDK client。
func (s *Store) OpenAccountClient(ctx context.Context, userID int64, options pixiv.Options) (*pixiv.Client, error) {
	session, err := s.openReadSession(ctx, userID, options)
	return session.Client, err
}

func (s *Store) openReadSession(ctx context.Context, userID int64, options pixiv.Options) (ReadSession, error) {
	// CAS 冲突后丢弃本次 OAuth 结果，重新读取获胜者的凭据再打开；OAuth 始终在锁外。
	for {
		if err := ctx.Err(); err != nil {
			return ReadSession{}, err
		}
		a, err := s.GetPixiv(ctx, userID)
		if err != nil {
			return ReadSession{}, fmt.Errorf("select pixiv account: %w", err)
		}
		client, credentials, err := pixiv.OpenWith(ctx, a.Credential.Value(), options)
		if err != nil {
			return ReadSession{}, err
		}
		if err := verifyAccountIdentity(userID, credentials.UserID); err != nil {
			client.CloseIdleConnections()
			return ReadSession{}, err
		}
		if err := s.RotatePixivCredentials(ctx, userID, a.CredentialRevision, []byte(credentials.RefreshToken())); err != nil {
			client.CloseIdleConnections()
			if errors.Is(err, ErrCredentialConflict) {
				continue
			}
			return ReadSession{}, fmt.Errorf("persist rotated pixiv credentials: %w", err)
		}
		return ReadSession{Client: client, Revision: a.CredentialRevision + 1, Expires: credentials.ExpiresAt}, nil
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
	client, credentials, err := pixiv.OpenWith(ctx, refreshToken, options)
	if err != nil {
		return Pixiv{}, err
	}
	defer client.CloseIdleConnections()
	return s.savePixivLogin(ctx, credentials)
}

func (s *Store) savePixivLogin(ctx context.Context, credentials pixiv.Credentials) (Pixiv, error) {
	a := Pixiv{UserID: credentials.UserID, Username: credentials.Username, Credential: credential.New([]byte(credentials.RefreshToken()))}
	if err := s.SavePixivCredential(ctx, a); err != nil {
		return Pixiv{}, fmt.Errorf("save pixiv account: %w", err)
	}
	saved, err := s.GetPixiv(ctx, a.UserID)
	if err != nil {
		return a, err
	}
	return saved, nil
}

// OpenFanboxClient 打开指定本地账号的旧 SDK client，不读取默认账号或网络配置。
func (s *Store) OpenFanboxClient(ctx context.Context, userID int64, options fanbox.Options) (*fanbox.Client, error) {
	a, err := s.GetFanbox(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("select fanbox account: %w", err)
	}
	return fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: a.Credential.Value()}, options)
}

// ImportFanbox 在线校验显式 session 并保存；Pixiv 凭据始终独立。
func (s *Store) ImportFanbox(ctx context.Context, session string, options fanbox.Options) (Fanbox, error) {
	client, err := fanbox.OpenWith(fanbox.SessionCredentials{FANBOXSESSID: session}, options)
	if err != nil {
		return Fanbox{}, err
	}
	defer client.CloseIdleConnections()
	user, err := client.CurrentUser(ctx, fanbox.CurrentUserRequest{})
	if err != nil {
		return Fanbox{}, err
	}
	a := Fanbox{UserID: user.UserID, DisplayName: user.DisplayName, CreatorID: user.CreatorID, Credential: credential.New([]byte(session)), ValidatedAt: time.Now().UTC()}
	if err := s.SaveFanboxCredential(ctx, a); err != nil {
		return Fanbox{}, fmt.Errorf("save fanbox account: %w", err)
	}
	return s.GetFanbox(ctx, a.UserID)
}
