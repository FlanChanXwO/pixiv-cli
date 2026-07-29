// Package protocol 集中维护 Pixiv 上游协议的静态约定。
//
// 它不读取配置、不执行网络请求；每个 Headers 函数都返回新的 map，调用方可以
// 安全地在本次请求上补充认证信息而不污染其他请求。
package protocol

const (
	AppAPIBase   = "https://app-api.pixiv.net"
	WebAPIBase   = "https://www.pixiv.net"
	OAuthBase    = "https://oauth.secure.pixiv.net"
	AppReferer   = AppAPIBase + "/"
	AppUserAgent = "PixivAndroidApp/5.0.234 (Android 11; Pixel 5)"
	WebUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
	AppOS        = "android"
	AppOSVersion = "11"
	AppVersion   = "5.0.234"

	OAuthClientID     = "MOBrBDS8blbauoSck0ZfDbtuzpyT"
	OAuthClientSecret = "lsACyCD94FhDUtGTXi3QzcFE2uU1hqtDaKeqrdwj"
	OAuthRedirectURI  = AppAPIBase + "/web/v1/users/auth/pixiv/callback"
)

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

func OAuthHeaders() map[string]string {
	headers := AppHeaders("")
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	return headers
}

func WebHeaders() map[string]string {
	return map[string]string{
		"User-Agent":      WebUserAgent,
		"Accept":          "application/json,text/plain,*/*",
		"Accept-Language": "zh-CN,zh;q=0.9,ja;q=0.8,en;q=0.7",
	}
}
