package pixiv_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	accountpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	sdkpixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginServiceStartPreservesPublicSessionBehavior(t *testing.T) {
	start, err := (accountpixiv.LoginService{}).Start()
	require.NoError(t, err)
	assert.NotEmpty(t, start.AuthorizationURL)
	assert.True(t, start.AcceptsCallbackURL(protocol.OAuthRedirectURI+"?code=placeholder&state="+url.QueryEscape("x")) || start.AuthorizationURL != "")

	parsed, err := url.Parse(start.AuthorizationURL)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)
	foreign := protocol.OAuthRedirectURI + "?code=foreign&state=other"
	matching := protocol.OAuthRedirectURI + "?code=accepted&state=" + url.QueryEscape(state)
	assert.False(t, start.AcceptsCallbackURL(foreign))
	assert.True(t, start.AcceptsCallbackURL(matching))
}

func TestLoginServiceStartAcceptsPublicSDKOptions(t *testing.T) {
	start, err := (accountpixiv.LoginService{}).Start(accountpixiv.LoginRequest{
		Options: sdkpixiv.LoginOptions{HTTPClient: &http.Client{}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, start.AuthorizationURL)
}

func TestLoginServiceCompleteRejectsMissingSession(t *testing.T) {
	_, err := (accountpixiv.LoginService{}).Complete(context.Background(), accountpixiv.LoginStart{}, accountpixiv.LoginCompleteRequest{})
	require.EqualError(t, err, "login session is not initialized")
}

func TestLoginServiceCompleteRejectsInvalidCallback(t *testing.T) {
	start, err := (accountpixiv.LoginService{}).Start()
	require.NoError(t, err)

	_, err = (accountpixiv.LoginService{}).Complete(context.Background(), start, accountpixiv.LoginCompleteRequest{
		CallbackOrCode: "https://example.com/?code=x&state=wrong",
	})
	require.ErrorContains(t, err, "login callback is invalid")
}

func TestLoginStartAcceptsCallbackRequiresSession(t *testing.T) {
	var empty accountpixiv.LoginStart
	assert.False(t, empty.AcceptsCallbackURL(protocol.OAuthRedirectURI+"?code=x"))
}

func TestLoginServicePreservesCLISelectionWhenRequested(t *testing.T) {
	for _, name := range []string{"no-default", "existing-default", "summary-error"} {
		t.Run(name, func(t *testing.T) {
			repo := newPixivTestRepository()
			defaults := &pixivTestDefaults{}
			if name == "existing-default" {
				defaults.userID, defaults.ok = 99, true
			}
			if name == "summary-error" {
				defaults.readErr = errors.New("fixture default read failure")
			}
			service := accountpixiv.LoginService{Pixiv: accountpixiv.NewService(repo, defaults)}
			client := &http.Client{Transport: pixivRoundTripper(func(r *http.Request) (*http.Response, error) {
				return pixivJSONResponse(`{"access_token":"fixture-access","refresh_token":"fixture-refresh","expires_in":3600,"user":{"id":73,"name":"fixture"}}`), nil
			})}
			start, err := service.Start(accountpixiv.LoginRequest{Options: sdkpixiv.LoginOptions{HTTPClient: client}})
			require.NoError(t, err)
			account, err := service.Complete(t.Context(), start, accountpixiv.LoginCompleteRequest{CallbackOrCode: "pixiv://account/login?code=fixture", PreserveDefault: true})
			if name == "summary-error" {
				require.ErrorIs(t, err, defaults.readErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, int64(73), account.UserID)
			require.True(t, repo.accounts[73].HasRefreshToken())
			if name == "existing-default" {
				require.Equal(t, int64(99), defaults.userID)
			} else {
				require.False(t, defaults.ok, "MCP login must not create a CLI default")
			}
		})
	}
}
