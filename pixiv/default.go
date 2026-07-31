package pixiv

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/oauth"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/credentials"
)

type defaultOptions struct {
	options   clientOptions
	authState *authTransactionState
}

type localSnapshot struct {
	authPath   string
	configPath string
	runtime    config.RuntimeConfig
	store      auth.AuthStore
}

type localStateStage string

const (
	// defaultPremiumStatusCacheTTL 保持 v0.7 已公布的一天缓存窗口，但不再暴露为
	// 可写配置；这样不会以无依据的短时限打断正常认证或搜索路径。
	defaultPremiumStatusCacheTTL                 = 24 * time.Hour
	localStateStagePath          localStateStage = "path"
	localStateStageAuth          localStateStage = "auth"
	localStateStageConfig        localStateStage = "config"
	localStateStageProxy         localStateStage = "proxy"
)

type localStateFailure struct {
	stage localStateStage
	err   error
}

func (e *localStateFailure) Error() string { return e.err.Error() }
func (e *localStateFailure) Unwrap() error { return e.err }

func markLocalState(stage localStateStage, err error) error {
	if err == nil {
		return nil
	}
	return &localStateFailure{stage: stage, err: err}
}

func (c *Client) operationClient(ctx context.Context, operation Operation) (*Client, error) {
	if c.defaults == nil {
		return c, nil
	}
	scoped, err := c.defaults.snapshot(ctx, operation)
	return scoped, err
}

func (d *defaultOptions) paths() (string, string, error) {
	authPath := strings.TrimSpace(d.options.AuthFilePath)
	if authPath == "" {
		var err error
		authPath, err = auth.AuthFilePath()
		if err != nil {
			return "", "", markLocalState(localStateStagePath, err)
		}
	}
	configPath := strings.TrimSpace(d.options.ConfigFilePath)
	if configPath == "" {
		var err error
		configPath, err = config.ConfigFilePath()
		if err != nil {
			return "", "", markLocalState(localStateStagePath, err)
		}
	}
	return authPath, configPath, nil
}

func (d *defaultOptions) loadSnapshot() (localSnapshot, error) {
	authPath, configPath, err := d.paths()
	if err != nil {
		return localSnapshot{}, err
	}
	state, err := config.LoadSettingsStateAt(configPath)
	if err != nil {
		return localSnapshot{}, markLocalState(localStateStageConfig, err)
	}
	runtime, err := state.Runtime()
	if err != nil {
		return localSnapshot{}, markLocalState(localStateStageConfig, err)
	}
	store, err := auth.LoadAuthStore(authPath)
	if err != nil {
		return localSnapshot{}, markLocalState(localStateStageAuth, err)
	}
	return localSnapshot{authPath: authPath, configPath: configPath, runtime: runtime, store: store}, nil
}

func (d *defaultOptions) snapshot(ctx context.Context, operation Operation) (*Client, error) {
	d.authState.mu.Lock()
	defer d.authState.mu.Unlock()
	snapshot, err := d.loadSnapshot()
	if err != nil {
		return nil, localSnapshotError(operation, err)
	}
	refreshToken, selectedUserID, selectedStored, err := d.selectRefreshToken(snapshot.store)
	if err != nil {
		if errors.Is(err, credentials.ErrCookieRefreshTokenInput) {
			return nil, newError(CodeInvalidArgument, operation, "", false, 0, 0, err)
		}
		return nil, newUserError(CodeInvalidArgument, operation, "", false, 0, d.options.UserID, errors.New("selected account does not exist"))
	}
	httpClient, err := newHTTPClientForSnapshot(d.options, snapshot.runtime.HTTPSProxy)
	if err != nil {
		return nil, localSnapshotError(operation, markLocalState(localStateStageProxy, err))
	}
	options := d.options
	options.HTTPClient = httpClient
	options.AccessToken = ""
	options.WebFallbackEnabled = snapshot.runtime.WebFallbackEnabled
	if refreshToken == "" {
		client, err := newClient(options)
		if err != nil {
			return nil, localSnapshotError(operation, err)
		}
		client.authState = d.authState
		client.cursorSource = "web:anonymous"
		return client, nil
	}

	oauthClient := oauth.New(refreshToken, oauth.WithHTTPClient(httpClient), oauth.WithBaseURL(d.options.OAuthBaseURL))
	if err := oauthClient.Refresh(ctx); err != nil {
		return nil, mapOAuthError(err, operation)
	}
	if oauthClient.UserID() <= 0 || strings.TrimSpace(oauthClient.AccessToken()) == "" {
		return nil, newError(CodeMalformedUpstreamResponse, operation, BackendOAuth, false, 0, 0, errors.New("oauth response did not include authenticated identity"))
	}
	if selectedStored {
		if oauthClient.UserID() != selectedUserID {
			return nil, accountMismatchError(operation, selectedUserID)
		}
		index, stored, ok := snapshot.store.Get(selectedUserID)
		if !ok {
			return nil, newUserError(CodeInvalidArgument, operation, "", false, 0, selectedUserID, errors.New("selected account does not exist"))
		}
		stored.RefreshToken = oauthClient.RefreshTokenValue()
		if username := strings.TrimSpace(oauthClient.UserName()); username != "" {
			stored.Username = username
		}
		snapshot.store.Accounts[index] = stored
		if err := auth.SaveAuthStore(snapshot.authPath, snapshot.store); err != nil {
			return nil, localSnapshotError(operation, markLocalState(localStateStageAuth, err))
		}
	}
	options.AccessToken = oauthClient.AccessToken()
	client, err := newClient(options)
	if err != nil {
		return nil, localSnapshotError(operation, err)
	}
	sourceUserID := oauthClient.UserID()
	if selectedStored {
		sourceUserID = selectedUserID
	}
	client.authState = d.authState
	client.cursorSource = "app:user:" + formatUserID(sourceUserID)
	client.authenticatedUserID = oauthClient.UserID()
	client.premiumStatusCacheTTL = defaultPremiumStatusCacheTTL
	if selectedStored {
		client.premiumStatusAuthPath = snapshot.authPath
		if _, stored, ok := snapshot.store.Get(selectedUserID); ok {
			if stored.PremiumStatus != nil {
				premium := *stored.PremiumStatus
				client.cachedPremiumStatus = &premium
			}
			if stored.PremiumStatusCheckedAt != nil {
				client.premiumStatusCheckedAt = *stored.PremiumStatusCheckedAt
			}
		}
	}
	return client, nil
}

func (d *defaultOptions) selectRefreshToken(store auth.AuthStore) (string, int64, bool, error) {
	if token := strings.TrimSpace(d.options.RefreshToken); token != "" {
		token, err := credentials.ValidateRefreshTokenInput(token)
		return token, 0, false, err
	}
	if d.options.UserID != 0 {
		if _, account, ok := store.Get(d.options.UserID); ok {
			token, err := credentials.ValidateRefreshTokenInput(account.RefreshToken)
			return token, account.UserID, true, err
		}
		return "", 0, false, errors.New("selected account does not exist")
	}
	if !d.options.IgnoreEnvironmentRefreshToken {
		if token, err := config.RefreshTokenFromEnv(); err != nil {
			return "", 0, false, err
		} else if token != "" {
			return token, 0, false, nil
		}
	}
	if userID, account, ok := auth.SelectAuthAccount(store, 0); ok {
		token, err := credentials.ValidateRefreshTokenInput(account.RefreshToken)
		return token, userID, true, err
	}
	return "", 0, false, nil
}

// resourceSnapshot reads current config/proxy state without touching OAuth or
// auth.json: resource fetching never needs an App credential.
func (d *defaultOptions) resourceSnapshot(operation Operation) (*Client, error) {
	_, configPath, err := d.paths()
	if err != nil {
		return nil, localSnapshotError(operation, err)
	}
	settings, err := config.LoadSettingsStateAt(configPath)
	if err != nil {
		return nil, localSnapshotError(operation, markLocalState(localStateStageConfig, err))
	}
	runtime, err := settings.Runtime()
	if err != nil {
		return nil, localSnapshotError(operation, markLocalState(localStateStageConfig, err))
	}
	httpClient, err := newHTTPClientForSnapshot(d.options, runtime.HTTPSProxy)
	if err != nil {
		return nil, localSnapshotError(operation, markLocalState(localStateStageProxy, err))
	}
	options := d.options
	options.AccessToken = ""
	options.HTTPClient = httpClient
	client, err := newClient(options)
	if err != nil {
		return nil, localSnapshotError(operation, err)
	}
	return client, nil
}

func localSnapshotError(operation Operation, err error) error {
	kind, cause := classifyLocalStateError(err)
	mapped := newError(CodeInvalidArgument, operation, "", false, 0, 0, cause)
	mapped.LocalStateKind = kind
	return mapped
}

func accountMismatchError(operation Operation, userID int64) error {
	mapped := newUserError(CodeInvalidArgument, operation, BackendOAuth, false, 0, userID, errors.New("oauth identity does not match selected account"))
	mapped.LocalStateKind = LocalStateKindAccountMismatch
	return mapped
}

func classifyLocalStateError(err error) (LocalStateKind, error) {
	if errors.Is(err, fs.ErrPermission) {
		return LocalStateKindPermissionDenied, errors.New("local state permission denied")
	}
	if errors.Is(err, fs.ErrNotExist) {
		return LocalStateKindNotFound, errors.New("local state was not found")
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return LocalStateKindUnavailable, errors.New("local state is unavailable")
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return LocalStateKindUnavailable, errors.New("local state is unavailable")
	}
	var syscallErr *os.SyscallError
	if errors.As(err, &syscallErr) {
		return LocalStateKindUnavailable, errors.New("local state is unavailable")
	}
	// syscall.Errno 是标准库对操作系统 syscall 状态的 typed 表示；只匹配该
	// 具体类型及其 wrapper，不能把普通业务 sentinel 误判为本地 I/O 失败。
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return LocalStateKindUnavailable, errors.New("local state is unavailable")
	}
	var failure *localStateFailure
	if errors.As(err, &failure) {
		switch failure.stage {
		case localStateStageAuth:
			return LocalStateKindAuthMalformed, errors.New("local authentication is malformed")
		case localStateStageConfig:
			return LocalStateKindConfigMalformed, errors.New("local configuration is malformed")
		case localStateStageProxy:
			return LocalStateKindInvalidProxy, errors.New("configured proxy URL is invalid")
		case localStateStagePath:
			return LocalStateKindUnavailable, errors.New("local state is unavailable")
		}
	}
	return LocalStateKindUnknown, errors.New("local authentication or configuration state is invalid")
}

func mapOAuthError(err error, operation Operation) error {
	return mapAdapterFailure(err, operation, BackendOAuth, 0, 0)
}

func formatUserID(id int64) string {
	// decimal is a non-secret stable identity suitable for cursor source binding.
	return strconv.FormatInt(id, 10)
}
