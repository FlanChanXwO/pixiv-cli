package application

// ClientRequest 是 CLI 解析 flags 后的本地请求值。内容与下载命令会进一步转换为
// SDKClientRequest；本类型不持有 Pixiv client，也不建立第二条认证或内容调用链。
type ClientRequest struct {
	UserID             int64
	RefreshToken       string
	HTTPSProxyOverride *string
	JSONOverride       *bool
	NeedsAuth          bool
}

// Services 只组合应用层的本地适配与 public SDK facade。
type Services struct {
	Account  AccountService
	Config   ConfigService
	Login    LoginService
	SDK      SDKService
	Download DownloadService
}
