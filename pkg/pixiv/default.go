package pixiv

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/oauth"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
)

type defaultOptions struct {
	options   Options
	authState *authTransactionState
}

type localSnapshot struct {
	authPath   string
	configPath string
	runtime    config.RuntimeConfig
	store      auth.AuthStore
}

func (c *Client) operationClient(ctx context.Context, operation Operation) (*Client, error) {
	if c.defaults == nil {
		return c, nil
	}
	started := time.Now()
	scoped, err := c.defaults.snapshot(ctx, operation)
	if err != nil {
		// operationClient 是 OpenDefault 建立一次真实 operation snapshot 的唯一入口；
		// 在此记录失败，避免外层 delegated public method 与 scoped method 双写。
		c.operationLog(operation, started, err, 0, 0)
	}
	return scoped, err
}

func (d *defaultOptions) paths() (string, string, error) {
	authPath := strings.TrimSpace(d.options.AuthFilePath)
	if authPath == "" {
		var err error
		authPath, err = auth.AuthFilePath()
		if err != nil {
			return "", "", err
		}
	}
	configPath := strings.TrimSpace(d.options.ConfigFilePath)
	if configPath == "" {
		var err error
		configPath, err = config.ConfigFilePath()
		if err != nil {
			return "", "", err
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
		return localSnapshot{}, err
	}
	runtime, err := state.Runtime()
	if err != nil {
		return localSnapshot{}, err
	}
	store, err := auth.LoadAuthStore(authPath)
	if err != nil {
		return localSnapshot{}, err
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
		return nil, newUserError(CodeInvalidArgument, operation, "", false, 0, d.options.UserID, errors.New("selected account does not exist"))
	}
	httpClient, err := newHTTPClientForSnapshot(d.options, snapshot.runtime.HTTPSProxy)
	if err != nil {
		return nil, localSnapshotError(operation, err)
	}
	options := d.options
	options.HTTPClient = httpClient
	options.AccessToken = ""
	options.WebFallbackEnabled = snapshot.runtime.WebFallbackEnabled
	if refreshToken == "" {
		client, err := NewClient(options)
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
			return nil, newUserError(CodeInvalidArgument, operation, BackendOAuth, false, 0, selectedUserID, errors.New("oauth identity does not match selected account"))
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
			return nil, localSnapshotError(operation, err)
		}
	}
	options.AccessToken = oauthClient.AccessToken()
	client, err := NewClient(options)
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
	return client, nil
}

func (d *defaultOptions) selectRefreshToken(store auth.AuthStore) (string, int64, bool, error) {
	if token := strings.TrimSpace(d.options.RefreshToken); token != "" {
		return token, 0, false, nil
	}
	if d.options.UserID != 0 {
		if _, account, ok := store.Get(d.options.UserID); ok {
			return account.RefreshToken, account.UserID, true, nil
		}
		return "", 0, false, errors.New("selected account does not exist")
	}
	if token := config.RefreshTokenFromEnv(); token != "" {
		return token, 0, false, nil
	}
	if userID, account, ok := auth.SelectAuthAccount(store, 0); ok {
		return account.RefreshToken, userID, true, nil
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
		return nil, localSnapshotError(operation, err)
	}
	runtime, err := settings.Runtime()
	if err != nil {
		return nil, localSnapshotError(operation, err)
	}
	httpClient, err := newHTTPClientForSnapshot(d.options, runtime.HTTPSProxy)
	if err != nil {
		return nil, localSnapshotError(operation, err)
	}
	options := d.options
	options.AccessToken = ""
	options.HTTPClient = httpClient
	client, err := NewClient(options)
	if err != nil {
		return nil, localSnapshotError(operation, err)
	}
	return client, nil
}

func localSnapshotError(operation Operation, _ error) error {
	return newError(CodeInvalidArgument, operation, "", false, 0, 0, errors.New("local authentication or configuration state is invalid"))
}

func mapOAuthError(err error, operation Operation) error {
	var upstream oauth.APIError
	if errors.As(err, &upstream) {
		code, retryable := codeForHTTPStatus(upstream.StatusCode, operation)
		return newError(code, operation, BackendOAuth, retryable, upstream.StatusCode, 0, errors.New("oauth upstream rejected the request"))
	}
	if errors.Is(err, oauth.ErrMalformedResponse) {
		return newError(CodeMalformedUpstreamResponse, operation, BackendOAuth, false, 0, 0, errors.New("oauth response was malformed"))
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return newError(CodeUpstreamUnavailable, operation, BackendOAuth, false, 0, 0, err)
	}
	return newError(CodeUpstreamUnavailable, operation, BackendOAuth, true, 0, 0, errors.New("oauth token refresh failed"))
}

func formatUserID(id int64) string {
	// decimal is a non-secret stable identity suitable for cursor source binding.
	return strconv.FormatInt(id, 10)
}
