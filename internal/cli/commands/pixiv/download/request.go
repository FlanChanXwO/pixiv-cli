package download

import (
	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
)

// ToPixivRequest 把 download command 的本地请求映射为共享 Pixiv SDK 请求。
func ToPixivRequest(request CommandRequest) deps.Request {
	return deps.Request{
		HTTPSProxyOverride: request.HTTPSProxyOverride,
	}
}
