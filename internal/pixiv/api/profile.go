package api

// AppAPIBase 与 OAuthBase 是 Pixiv App 与 OAuth 的固定入口，其余常量是请求头基线。
const (
	AppAPIBase   = "https://app-api.pixiv.net"
	OAuthBase    = "https://oauth.secure.pixiv.net"
	AppReferer   = AppAPIBase + "/"
	AppUserAgent = "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)"
	AppOS        = "android"
	AppOSVersion = "11"
	AppVersion   = "5.0.234"

	OAuthClientID     = "MOBrBDS8blbauoSck0ZfDbtuzpyT"
	OAuthClientSecret = "lsACyCD94FhDUtGTXi3QzcFE2uU1hqtDaKeqrdwj"
	OAuthRedirectURI  = AppAPIBase + "/web/v1/users/auth/pixiv/callback"
)

// AppHeaders 返回每次请求新建的 App 请求头；accessToken 非空时附带 Bearer 认证。
func AppHeaders(accessToken string) map[string]string {
	headers := map[string]string{
		"User-Agent":     AppUserAgent,
		"App-OS":         AppOS,
		"App-OS-Version": AppOSVersion,
		"App-Version":    AppVersion,
		"Referer":        AppReferer,
	}
	if accessToken != "" {
		headers["Authorization"] = "Bearer " + accessToken
	}
	return headers
}

// OAuthHeaders 返回表单提交使用的请求头，包含 form Content-Type，不带认证。
func OAuthHeaders() map[string]string {
	headers := AppHeaders("")
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	return headers
}
