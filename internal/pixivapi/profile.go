// Pixiv protocol constants and fresh per-request App/OAuth header maps.

package pixivapi

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
