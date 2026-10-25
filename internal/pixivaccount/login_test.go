package pixivaccount_test

import (
	"context"
	"errors"
	pixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixivaccount"
	protocol "github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	sdkpixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/url"
	"testing"
)

func TestLoginServiceStartPreservesPublicSessionBehavior(t *testing.T) {
	start, err := (pixiv.LoginService{}).Start()
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

func TestLoginServiceCompleteRejectsMissingSession(t *testing.T) {
	_, err := (pixiv.LoginService{}).Complete(context.Background(), pixiv.LoginStart{}, pixiv.LoginCompleteRequest{})
	require.EqualError(t, err, "login session is not initialized")
}

func TestLoginServiceCompleteRejectsInvalidCallback(t *testing.T) {
	start, err := (pixiv.LoginService{}).Start()
	require.NoError(t, err)

	_, err = (pixiv.LoginService{}).Complete(context.Background(), start, pixiv.LoginCompleteRequest{
		CallbackOrCode: "https://example.com/?code=x&state=wrong",
	})
	require.ErrorContains(t, err, "login callback is invalid")
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
			service := pixiv.LoginService{Pixiv: pixiv.NewService(repo, defaults)}
			client := &http.Client{Transport: pixivRoundTripper(func(r *http.Request) (*http.Response, error) {
				return pixivJSONResponse(`{"access_token":"fixture-access","refresh_token":"fixture-refresh","expires_in":3600,"user":{"id":73,"name":"fixture"}}`), nil
			})}
			start, err := service.Start(pixiv.LoginRequest{Options: sdkpixiv.LoginOptions{HTTPClient: client}})
			require.NoError(t, err)
			account, err := service.Complete(t.Context(), start, pixiv.LoginCompleteRequest{CallbackOrCode: "pixiv://account/login?code=fixture", PreserveDefault: true})
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
