package pixiv

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/oauth"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/credentials"
)

// AuthFilePath 返回此 Client 操作账号数据的路径。显式 NewClient 必须提供
// AuthFilePath；OpenDefault 会在每次调用时解析现有默认路径。
func (c *Client) AuthFilePath() (string, error) {
	if c.defaults != nil {
		path, _, err := c.defaults.paths()
		if err != nil {
			return "", localSnapshotError(OperationListAccounts, err)
		}
		return path, nil
	}
	if c.authFilePath == "" {
		return "", unsupportedLocalStore(OperationListAccounts, "auth store path was not configured")
	}
	return c.authFilePath, nil
}

// ConfigFilePath 返回此 Client 操作配置数据的路径。显式 NewClient 必须提供
// ConfigFilePath；OpenDefault 会在每次调用时解析现有默认路径。
func (c *Client) ConfigFilePath() (string, error) {
	return c.configPathFor(OperationConfigGet)
}

func (c *Client) configPathFor(operation Operation) (string, error) {
	if c.defaults != nil {
		_, path, err := c.defaults.paths()
		if err != nil {
			return "", localSnapshotError(operation, err)
		}
		return path, nil
	}
	if c.configFilePath == "" {
		return "", unsupportedLocalStore(operation, "config store path was not configured")
	}
	return c.configFilePath, nil
}

// ImportAccount 刷新给定 refresh token，随后安全保存其旋转后的 token。
func (c *Client) ImportAccount(ctx context.Context, refreshToken string) (out *Account, err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationImportAccount, started, err, 0, 0) }()
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()
	refreshToken, inputErr := credentials.ValidateRefreshTokenInput(refreshToken)
	if inputErr != nil {
		return nil, newError(CodeInvalidArgument, OperationImportAccount, "", false, 0, 0, inputErr)
	}
	if refreshToken == "" {
		return nil, newError(CodeInvalidArgument, OperationImportAccount, "", false, 0, 0, errors.New("refresh token is required"))
	}
	state, httpClient, err := c.accountState(OperationImportAccount, true)
	if err != nil {
		return nil, err
	}
	account, err := c.refreshIdentity(ctx, OperationImportAccount, httpClient, refreshToken)
	if err != nil {
		return nil, err
	}
	state.store.Upsert(account)
	if state.store.DefaultUserID == 0 {
		state.store.DefaultUserID = account.UserID
	}
	if err := auth.SaveAuthStore(state.authPath, state.store); err != nil {
		return nil, localSnapshotError(OperationImportAccount, markLocalState(localStateStageAuth, err))
	}
	result := publicAccount(account, state.store.DefaultUserID)
	return &result, nil
}

// ListAccounts 返回本地账号的安全摘要。
func (c *Client) ListAccounts() (out *AccountsResult, err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationListAccounts, started, err, 0, 0) }()
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()
	state, _, err := c.accountState(OperationListAccounts, false)
	if err != nil {
		return nil, err
	}
	result := &AccountsResult{DefaultUserID: state.store.DefaultUserID, Accounts: make([]Account, len(state.store.Accounts))}
	for i, account := range state.store.Accounts {
		result.Accounts[i] = publicAccount(account, state.store.DefaultUserID)
	}
	return result, nil
}

// ExportAccountRefreshToken 返回本地账号保存的 refresh token。该显式导出操作只
// 读取 auth store，不应用环境变量或运行时账号优先级，也不会联网、刷新或改写凭据。
func (c *Client) ExportAccountRefreshToken(userID int64) (token string, err error) {
	// 这是唯一会把 refresh token 作为返回值交给调用方的公开操作；即使日志内容
	// 本身已脱敏，成功事件也会污染 CLI 的凭据专用 stderr，因此此操作保持静默。
	if userID < 0 {
		return "", newError(CodeInvalidArgument, OperationExportAccountRefreshToken, "", false, 0, 0, errors.New("user id must be positive"))
	}
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()

	authPath := c.authFilePath
	if c.defaults != nil {
		authPath = strings.TrimSpace(c.defaults.options.AuthFilePath)
		if authPath == "" {
			authPath, err = auth.AuthFilePath()
			if err != nil {
				return "", localSnapshotError(OperationExportAccountRefreshToken, markLocalState(localStateStagePath, err))
			}
		}
	}
	if authPath == "" {
		return "", unsupportedLocalStore(OperationExportAccountRefreshToken, "auth store path was not configured")
	}
	store, loadErr := auth.LoadAuthStore(authPath)
	if loadErr != nil {
		targetUserID := userID
		if targetUserID == 0 {
			targetUserID = store.DefaultUserID
		}
		_, selected, selectedOK := store.Get(targetUserID)
		if !auth.IsMissingRefreshTokenError(loadErr) || !selectedOK || strings.TrimSpace(selected.RefreshToken) != "" {
			return "", localSnapshotError(OperationExportAccountRefreshToken, markLocalState(localStateStageAuth, loadErr))
		}
	}
	selectedUserID, account, ok := auth.SelectAuthAccount(store, userID)
	if !ok {
		if userID == 0 {
			return "", newError(CodeUnauthorized, OperationExportAccountRefreshToken, "", false, 0, 0, errors.New("a stored account is required"))
		}
		return "", newUserError(CodeInvalidArgument, OperationExportAccountRefreshToken, "", false, 0, userID, errors.New("account does not exist"))
	}
	token, inputErr := credentials.ValidateRefreshTokenInput(account.RefreshToken)
	if inputErr != nil {
		return "", newUserError(CodeInvalidArgument, OperationExportAccountRefreshToken, "", false, 0, selectedUserID, inputErr)
	}
	if token == "" {
		return "", newUserError(CodeUnauthorized, OperationExportAccountRefreshToken, "", false, 0, selectedUserID, errors.New("account token is unavailable"))
	}
	return token, nil
}

// SelectAccount 将已有账号选为默认账号。
func (c *Client) SelectAccount(userID int64) (err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationSelectAccount, started, err, 0, userID) }()
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()
	if userID <= 0 {
		return newError(CodeInvalidArgument, OperationSelectAccount, "", false, 0, 0, errors.New("user id must be positive"))
	}
	state, _, err := c.accountState(OperationSelectAccount, false)
	if err != nil {
		return err
	}
	if !state.store.Has(userID) {
		return newUserError(CodeInvalidArgument, OperationSelectAccount, "", false, 0, userID, errors.New("account does not exist"))
	}
	state.store.DefaultUserID = userID
	if err := auth.SaveAuthStore(state.authPath, state.store); err != nil {
		return localSnapshotError(OperationSelectAccount, markLocalState(localStateStageAuth, err))
	}
	return nil
}

// RemoveAccount 删除本地账号；删除默认账号时沿用存储层的确定性提升规则。
func (c *Client) RemoveAccount(userID int64) (err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationRemoveAccount, started, err, 0, userID) }()
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()
	if userID <= 0 {
		return newError(CodeInvalidArgument, OperationRemoveAccount, "", false, 0, 0, errors.New("user id must be positive"))
	}
	state, _, err := c.accountState(OperationRemoveAccount, false)
	if err != nil {
		return err
	}
	if !state.store.Remove(userID) {
		return newUserError(CodeInvalidArgument, OperationRemoveAccount, "", false, 0, userID, errors.New("account does not exist"))
	}
	if err := auth.SaveAuthStore(state.authPath, state.store); err != nil {
		return localSnapshotError(OperationRemoveAccount, markLocalState(localStateStageAuth, err))
	}
	return nil
}

// CheckAccount 刷新指定本地账号，验证 UID 不变并保存旋转后的 token。
func (c *Client) CheckAccount(ctx context.Context, userID int64) (out *Account, err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationCheckAccount, started, err, 0, userID) }()
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()
	state, httpClient, err := c.accountState(OperationCheckAccount, true)
	if err != nil {
		return nil, err
	}
	if userID == 0 {
		userID = state.store.DefaultUserID
	}
	return c.refreshStoredAccountFromState(ctx, OperationCheckAccount, userID, state, httpClient)
}

// CheckRefreshToken 验证一次调用方提供的 refresh token 并返回其身份摘要。它不会读取、
// 选择、旋转或写入本地 auth store，因此环境变量等临时凭据不能改变默认账号。
func (c *Client) CheckRefreshToken(ctx context.Context, refreshToken string) (out *Account, err error) {
	started := time.Now()
	if c.defaults != nil {
		// 临时 token 只需要当前配置决定的 transport；不得走会选择并刷新本地账号的
		// operation snapshot，否则环境变量检查会意外改写 auth.json。
		scoped, snapshotErr := c.defaults.resourceSnapshot(OperationCheckRefreshToken)
		if snapshotErr != nil {
			c.operationLog(OperationCheckRefreshToken, started, snapshotErr, 0, 0)
			return nil, snapshotErr
		}
		out, err = scoped.checkRefreshToken(ctx, refreshToken)
		c.operationLog(OperationCheckRefreshToken, started, err, 0, accountUserID(out))
		return out, err
	}
	defer func() { c.operationLog(OperationCheckRefreshToken, started, err, 0, accountUserID(out)) }()
	return c.checkRefreshToken(ctx, refreshToken)
}

func (c *Client) checkRefreshToken(ctx context.Context, refreshToken string) (*Account, error) {
	refreshToken, inputErr := credentials.ValidateRefreshTokenInput(refreshToken)
	if inputErr != nil {
		return nil, newError(CodeInvalidArgument, OperationCheckRefreshToken, "", false, 0, 0, inputErr)
	}
	if refreshToken == "" {
		return nil, newError(CodeInvalidArgument, OperationCheckRefreshToken, "", false, 0, 0, errors.New("refresh token is required"))
	}
	account, err := c.refreshIdentity(ctx, OperationCheckRefreshToken, c.httpClient, refreshToken)
	if err != nil {
		return nil, err
	}
	result := publicAccount(account, 0)
	return &result, nil
}

func accountUserID(account *Account) int64 {
	if account == nil {
		return 0
	}
	return account.UserID
}

// Refresh 刷新当前默认账号并保存旋转后的 token。
func (c *Client) Refresh(ctx context.Context) (out *Account, err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationRefresh, started, err, 0, 0) }()
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()
	state, httpClient, err := c.accountState(OperationRefresh, true)
	if err != nil {
		return nil, err
	}
	return c.refreshStoredAccountFromState(ctx, OperationRefresh, state.store.DefaultUserID, state, httpClient)
}

// RefreshAccount 是 Refresh 的显式 UID 变体。
func (c *Client) RefreshAccount(ctx context.Context, userID int64) (out *Account, err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationRefresh, started, err, 0, userID) }()
	c.authState.mu.Lock()
	defer c.authState.mu.Unlock()
	return c.refreshStoredAccount(ctx, OperationRefresh, userID)
}

func (c *Client) refreshStoredAccount(ctx context.Context, operation Operation, userID int64) (*Account, error) {
	if userID <= 0 {
		return nil, newError(CodeUnauthorized, operation, "", false, 0, 0, errors.New("a stored account is required"))
	}
	state, httpClient, err := c.accountState(operation, true)
	if err != nil {
		return nil, err
	}
	return c.refreshStoredAccountFromState(ctx, operation, userID, state, httpClient)
}

func (c *Client) refreshStoredAccountFromState(ctx context.Context, operation Operation, userID int64, state localSnapshot, httpClient *http.Client) (*Account, error) {
	index, stored, ok := state.store.Get(userID)
	if !ok || strings.TrimSpace(stored.RefreshToken) == "" {
		return nil, newUserError(CodeUnauthorized, operation, "", false, 0, userID, errors.New("account token is unavailable"))
	}
	refreshToken, inputErr := credentials.ValidateRefreshTokenInput(stored.RefreshToken)
	if inputErr != nil {
		return nil, newUserError(CodeInvalidArgument, operation, "", false, 0, userID, inputErr)
	}
	updated, err := c.refreshIdentity(ctx, operation, httpClient, refreshToken)
	if err != nil {
		return nil, err
	}
	if updated.UserID != stored.UserID {
		return nil, accountMismatchError(operation, userID)
	}
	state.store.Accounts[index] = updated
	if err := auth.SaveAuthStore(state.authPath, state.store); err != nil {
		return nil, localSnapshotError(operation, markLocalState(localStateStageAuth, err))
	}
	result := publicAccount(updated, state.store.DefaultUserID)
	return &result, nil
}

func (c *Client) refreshIdentity(ctx context.Context, operation Operation, httpClient *http.Client, refreshToken string) (auth.Account, error) {
	oauthClient := oauth.New(refreshToken, oauth.WithHTTPClient(httpClient), oauth.WithBaseURL(c.oauthBase()))
	if err := oauthClient.Refresh(ctx); err != nil {
		return auth.Account{}, mapOAuthError(err, operation)
	}
	if oauthClient.UserID() <= 0 || strings.TrimSpace(oauthClient.RefreshTokenValue()) == "" {
		return auth.Account{}, newError(CodeMalformedUpstreamResponse, operation, BackendOAuth, false, 0, 0, errors.New("oauth response did not include account identity"))
	}
	return auth.Account{UserID: oauthClient.UserID(), Username: strings.TrimSpace(oauthClient.UserName()), RefreshToken: oauthClient.RefreshTokenValue()}, nil
}

func (c *Client) oauthBase() string {
	if c.defaults != nil {
		return c.defaults.options.OAuthBaseURL
	}
	return c.oauthBaseURL
}

func publicAccount(account auth.Account, defaultUserID int64) Account {
	return Account{UserID: account.UserID, Username: account.Username, Default: account.UserID == defaultUserID, HasToken: strings.TrimSpace(account.RefreshToken) != ""}
}

func unsupportedLocalStore(operation Operation, message string) error {
	return newError(CodeUnsupported, operation, "", false, 0, 0, errors.New(message))
}

func (c *Client) accountState(operation Operation, needsHTTP bool) (localSnapshot, *http.Client, error) {
	if c.defaults != nil {
		state, err := c.defaults.loadSnapshot()
		if err != nil {
			return localSnapshot{}, nil, localSnapshotError(operation, err)
		}
		if !needsHTTP {
			return state, nil, nil
		}
		httpClient, err := newHTTPClientForSnapshot(c.defaults.options, state.runtime.HTTPSProxy)
		if err != nil {
			return localSnapshot{}, nil, localSnapshotError(operation, markLocalState(localStateStageProxy, err))
		}
		return state, httpClient, nil
	}
	if c.authFilePath == "" {
		return localSnapshot{}, nil, unsupportedLocalStore(operation, "auth store path was not configured")
	}
	store, err := auth.LoadAuthStore(c.authFilePath)
	if err != nil {
		return localSnapshot{}, nil, localSnapshotError(operation, markLocalState(localStateStageAuth, err))
	}
	return localSnapshot{authPath: c.authFilePath, configPath: c.configFilePath, store: store}, c.httpClient, nil
}

// GetConfig 读取 alias 的有效值，包含现有环境变量覆盖规则。
func (c *Client) GetConfig(alias string) (out ConfigValue, err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationConfigGet, started, err, 0, 0) }()
	path, err := c.configPathFor(OperationConfigGet)
	if err != nil {
		return ConfigValue{}, err
	}
	state, err := config.LoadSettingsStateAt(path)
	if err != nil {
		return ConfigValue{}, localSnapshotError(OperationConfigGet, markLocalState(localStateStageConfig, err))
	}
	value, err := state.Effective(alias)
	if err != nil {
		return ConfigValue{}, newError(CodeInvalidArgument, OperationConfigGet, "", false, 0, 0, errors.New("config key is invalid"))
	}
	return ConfigValue{Key: alias, Value: value.Text, Source: value.Source, HasValue: value.HasValue}, nil
}

// SetConfig 校验并私有写入一个现有 alias。
func (c *Client) SetConfig(alias, raw string) (out ConfigValue, err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationConfigSet, started, err, 0, 0) }()
	path, err := c.configPathFor(OperationConfigSet)
	if err != nil {
		return ConfigValue{}, err
	}
	value, parsed, err := config.ParseSettingInput(alias, raw)
	if err != nil {
		return ConfigValue{}, newError(CodeInvalidArgument, OperationConfigSet, "", false, 0, 0, errors.New("config key or value is invalid"))
	}
	if err := config.SetConfigValue(path, alias, parsed); err != nil {
		return ConfigValue{}, localSnapshotError(OperationConfigSet, markLocalState(localStateStageConfig, err))
	}
	return ConfigValue{Key: alias, Value: value.Text, Source: "file", HasValue: true}, nil
}

// UnsetConfig 移除一个现有 alias 的文件值；环境覆盖仍会由 GetConfig 显示。
func (c *Client) UnsetConfig(alias string) (out bool, err error) {
	started := time.Now()
	defer func() { c.operationLog(OperationConfigUnset, started, err, 0, 0) }()
	path, err := c.configPathFor(OperationConfigUnset)
	if err != nil {
		return false, err
	}
	removed, err := config.UnsetConfigValue(path, alias)
	if err != nil {
		if _, ok := config.SettingSpecByAlias(alias); !ok {
			return false, newError(CodeInvalidArgument, OperationConfigUnset, "", false, 0, 0, errors.New("config key is invalid"))
		}
		return false, localSnapshotError(OperationConfigUnset, markLocalState(localStateStageConfig, err))
	}
	return removed, nil
}
