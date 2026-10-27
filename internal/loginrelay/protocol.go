package loginrelay

import (
	"net/url"
	"strings"
)

// RelayResultURLHeader 只承载一次性、无敏感最终页 URL。server 直到 OAuth
// exchange 完成才结束 callback response；client 在此期间打开结果页。
const RelayResultURLHeader = "X-Pixiv-Relay-Result-URL"

// RemoteLoginStartResponse 携带远程登录启动接口返回给桌面协议处理器的 JSON 形状。
type RemoteLoginStartResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}

func isPixivCallback(parsed *url.URL) bool {
	return parsed != nil && strings.EqualFold(parsed.Scheme, "pixiv") && strings.EqualFold(parsed.Host, "account") && parsed.Path == "/login" && strings.TrimSpace(parsed.Query().Get("code")) != ""
}

// IsAllowedPixivCallbackURL 按持久协议 handler 的精确白名单校验；上游若
// 增加可转发路径，应在这里以 host/path 规则明确扩展，不能把任意 pixiv:// URL
// 发给远程服务端。
func IsAllowedPixivCallbackURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && isPixivCallback(parsed)
}
