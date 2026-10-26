// FANBOX 媒体 URL 策略：允许的主机、是否需要会话 Cookie，以及资源 URL 校验。
// 媒体请求发送在 protocol.go，定位缓存由 fanboxapi.go 的 Client 持有。

package fanboxapi

import (
	"errors"
	"net/url"
	"strings"
)

// Referer is the non-secret referer required by FANBOX media requests.
const Referer = WebBaseURL

// DownloadsHost is the first-party FANBOX attachment host whose media URLs
// require the FANBOXSESSID cookie. Resources on this host are credentialed and
// callers must open them through OpenResource rather than treating the URL as
// a public link; Pixiv CDN and other allowlisted media hosts are public.
const DownloadsHost = "downloads.fanbox.cc"

// RequiresCredentials reports whether rawURL points at a credentialed FANBOX
// attachment host. It parses rawURL defensively and returns false on any parse
// error so callers treat unparseable locators as public (and let the host
// allowlist reject them later) rather than falsely advertising credentials.
func RequiresCredentials(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), DownloadsHost)
}

// ValidateURL validates a FANBOX asset locator without opening it.
func ValidateURL(rawURL string) error {
	parsed, err := parseAllowedURL(rawURL, requestKindMedia)
	if err != nil {
		return err
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return errors.New("FANBOX resource URL must contain a path")
	}
	return nil
}
