package pixivutil

import (
	"net/url"
	"strings"
)

// ParseRefreshTokenInput 接受原始 refresh token，或包含 refresh_token 键的 Cookie 字符串。
// Pixiv 网页 Cookie 中常见的 PHPSESSID/device_token 不是 App API 的 OAuth refresh token。
func ParseRefreshTokenInput(input string) (token string, parsedCookie bool) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", false
	}
	if token, ok := refreshTokenFromCookie(value); ok {
		return token, true
	}
	if looksLikeCookie(value) {
		return "", true
	}
	return value, false
}

func refreshTokenFromCookie(input string) (string, bool) {
	for _, part := range strings.Split(input, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || strings.TrimSpace(name) != "refresh_token" {
			continue
		}
		token := strings.TrimSpace(value)
		if decoded, err := url.PathUnescape(token); err == nil {
			token = decoded
		}
		return token, token != ""
	}
	return "", false
}

func looksLikeCookie(input string) bool {
	pairs := 0
	for _, part := range strings.Split(input, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if !ok || name == "" || value == "" {
			continue
		}
		if isPixivCookieName(name) {
			return true
		}
		pairs++
	}
	return pairs > 1
}

func isPixivCookieName(name string) bool {
	switch name {
	case "PHPSESSID", "device_token", "yuid_b", "p_ab_id", "p_ab_id_2", "privacy_policy_agreement", "privacy_policy_notification":
		return true
	default:
		return false
	}
}
