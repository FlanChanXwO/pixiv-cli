package pixiv_test

import (
	"context"
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
