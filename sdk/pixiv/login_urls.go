package pixiv

import (
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

// IsOfficialOAuthCallbackURL reports whether rawURL has the exact official
// Pixiv OAuth callback origin and callback path. It performs no network I/O.
func IsOfficialOAuthCallbackURL(rawURL string) bool {
	callback, err := url.Parse(protocol.OAuthRedirectURI)
	if err != nil {
		return false
	}
	return isOfficialOAuthURL(rawURL, callback.Path)
}

// IsOfficialOAuthStartURL reports whether rawURL has the exact official Pixiv
// App OAuth start origin and path. It performs no network I/O.
func IsOfficialOAuthStartURL(rawURL string) bool {
	return isOfficialOAuthURL(rawURL, protocol.AppOAuthStart)
}

func isOfficialOAuthURL(rawURL, expectedPath string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	callback, err := url.Parse(protocol.OAuthRedirectURI)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, callback.Scheme) && strings.EqualFold(parsed.Host, callback.Host) && parsed.Path == expectedPath
}
