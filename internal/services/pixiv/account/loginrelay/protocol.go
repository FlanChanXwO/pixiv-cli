// Package loginrelay owns the shared remote-login protocol, independent of CLI and OS helpers.
package loginrelay

import (
	"net/url"
	"strings"
)

// RelayResultURLHeader 只承载一次性、无敏感最终页 URL。server 直到 OAuth
// exchange 完成才结束 callback response；client 在此期间打开结果页。
const RelayResultURLHeader = "X-Pixiv-Relay-Result-URL"

type RemoteLoginStartResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}

func isPixivCallback(parsed *url.URL) bool {
	return parsed != nil && strings.EqualFold(parsed.Scheme, "pixiv") && strings.EqualFold(parsed.Host, "account") && parsed.Path == "/login" && strings.TrimSpace(parsed.Query().Get("code")) != ""
}

// IsAllowedPixivCallbackURL 是持久协议 handler 的精确白名单。未来若 Pixiv
// 增加可转发路径，应在这里以 host/path 规则明确扩展，不能把任意 pixiv:// URL
// 发给远程服务端。
func IsAllowedPixivCallbackURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && isPixivCallback(parsed)
}
