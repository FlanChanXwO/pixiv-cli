package pixiv

import (
	"context"
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/network"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginServiceStartPreservesPublicSessionBehavior(t *testing.T) {
	start, err := (LoginService{}).Start()
	require.NoError(t, err)
	assert.NotEmpty(t, start.AuthorizationURL)
	assert.NotNil(t, start.session)

	parsed, err := url.Parse(start.AuthorizationURL)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)
	foreign := protocol.OAuthRedirectURI + "?code=foreign&state=other"
	matching := protocol.OAuthRedirectURI + "?code=accepted&state=" + url.QueryEscape(state)
	assert.False(t, start.AcceptsCallbackURL(foreign))
	assert.True(t, start.AcceptsCallbackURL(matching))
}

func TestLoginServiceStartPropagatesProxyClientError(t *testing.T) {
	proxy := "http://proxy.invalid:7890"
	start, err := (LoginService{ProxyHTTPClient: network.HTTPClient}).Start(SDKClientRequest{HTTPSProxyOverride: &proxy})
	require.NoError(t, err)
	require.NotEmpty(t, start.AuthorizationURL)
}

func TestLoginServiceCompleteRejectsMissingSession(t *testing.T) {
	_, err := (LoginService{}).Complete(context.Background(), LoginStart{}, LoginCompleteRequest{})
	require.EqualError(t, err, "login session is not initialized")
}

func TestLoginServiceCompleteRejectsInvalidCallback(t *testing.T) {
	start, err := (LoginService{}).Start()
	require.NoError(t, err)

	_, err = (LoginService{}).Complete(context.Background(), start, LoginCompleteRequest{
		CallbackOrCode: "https://example.com/?code=x&state=wrong",
	})
	require.ErrorContains(t, err, "login callback is invalid")
}

func TestLoginStartAcceptsCallbackRequiresSession(t *testing.T) {
	var empty LoginStart
	assert.False(t, empty.AcceptsCallbackURL(protocol.OAuthRedirectURI+"?code=x"))
}
