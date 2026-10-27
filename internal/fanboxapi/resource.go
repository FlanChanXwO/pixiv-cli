package fanboxapi

import (
	"errors"
	"net/url"
	"strings"
)

// Referer 保存 FANBOX 媒体请求需要的非 secret Referer。
const Referer = WebBaseURL

// DownloadsHost 标识需要 FANBOXSESSID Cookie 的一手 FANBOX 附件主机。该主机的资源带
// 凭据，必须经 OpenResource 打开；Pixiv CDN 与其他允许的媒体主机是公开链接。
const DownloadsHost = "downloads.fanbox.cc"

// RequiresCredentials 报告 rawURL 是否指向需要凭据的 FANBOX 附件主机。解析失败时返回
// false，让调用方把无法解析的 locator 当作公开链接，后续由主机 allowlist 拒绝。
func RequiresCredentials(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), DownloadsHost)
}

// ValidateURL 只校验 FANBOX 资源 locator，不打开请求。
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
