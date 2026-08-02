package application

import (
	"time"

	fanboxapp "github.com/FlanChanXwO/pixiv-cli/internal/application/fanbox"
)

// ClientRequest 是 CLI 解析 flags 后的本地请求值。内容与下载命令会进一步转换为
// SDKClientRequest；本类型不持有 Pixiv client，也不建立第二条认证或内容调用链。
type ClientRequest struct {
	UserID                  int64
	RefreshToken            string
	HTTPSProxyOverride      *string
	RequestIntervalOverride *time.Duration
	JSONOverride            *bool
	NeedsAuth               bool
}

// Services 只组合应用层的本地适配与 public SDK facade。Fanbox 为 nil 时表示
// FANBOX 本地状态不可用，fanbox 命令必须给出明确错误而不是 panic。
type Services struct {
	Account  AccountService
	Config   ConfigService
	Login    LoginService
	SDK      SDKService
	Download DownloadService
	Fanbox   *fanboxapp.Service
}
