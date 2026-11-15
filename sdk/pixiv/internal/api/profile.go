package api

// AppAPIBase and OAuthBase identify the fixed endpoints; the remaining constants
// preserve the App request-header baseline.
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

// AppHeaders returns fresh App headers with Bearer authentication when provided.
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

// OAuthHeaders returns unauthenticated form headers for token exchange.
func OAuthHeaders() map[string]string {
	headers := AppHeaders("")
	headers["Content-Type"] = "application/x-www-form-urlencoded"
	return headers
}
