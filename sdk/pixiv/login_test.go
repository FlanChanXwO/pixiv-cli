package pixiv

import (
	"net/url"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	"github.com/stretchr/testify/require"
)

func TestLoginSessionAcceptsCallbackURLWithoutConsumingSession(t *testing.T) {
	session, err := BeginLogin(LoginOptions{})
	require.NoError(t, err)
	parsed, err := url.Parse(session.AuthorizationURL())
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	callback := protocol.OAuthRedirectURI + "?code=accepted&state=" + url.QueryEscape(state)

	require.True(t, session.AcceptsCallbackURL(callback))
	require.True(t, session.AcceptsCallbackURL(callback))
	require.False(t, session.AcceptsCallbackURL(protocol.OAuthRedirectURI+"?code=accepted&state=wrong"))
}

func TestOfficialOAuthURLValidators(t *testing.T) {
	require.True(t, IsOfficialOAuthCallbackURL(protocol.OAuthRedirectURI+"?code=x"))
	require.False(t, IsOfficialOAuthCallbackURL("https://example.invalid/web/v1/users/auth/pixiv/callback?code=x"))
	require.True(t, IsOfficialOAuthStartURL(protocol.AppAPIBase+protocol.AppOAuthStart+"?state=x"))
	require.False(t, IsOfficialOAuthStartURL(protocol.OAuthRedirectURI+"?state=x"))
}
