package utils

import (
	"errors"
	"strings"
)

// ErrCookieRefreshTokenInput 表示调用方误把网页 Cookie 当成 App API refresh token。
// 此错误不包含输入内容，避免凭据进入 CLI、MCP 或日志输出。
var ErrCookieRefreshTokenInput = errors.New("cookie input is not supported; provide a Pixiv App API refresh token")

// ValidateRefreshTokenInput 只接受 Pixiv App API 的不透明 refresh token。单个
// "x=y" 仍可能是合法 token；但 Cookie 的明确名称或多 cookie pair 一律拒绝，绝不
// 从中提取或转换 refresh_token。
func ValidateRefreshTokenInput(input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if looksLikeCookie(value) {
		return "", ErrCookieRefreshTokenInput
	}
	return value, nil
}

func looksLikeCookie(value string) bool {
	if strings.HasPrefix(strings.ToLower(value), "cookie:") {
		return true
	}

	pairs := 0
	for _, part := range strings.Split(value, ";") {
		name, token, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(token) == "" {
			continue
		}
		if isCookieName(name) {
			return true
		}
		pairs++
	}
	return pairs > 1
}

func isCookieName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "refresh_token", "phpsessid", "session", "sessionid", "session_id", "csrftoken", "csrf_token", "device_token", "yuid_b", "p_ab_id", "p_ab_id_2", "privacy_policy_agreement", "privacy_policy_notification":
		return true
	default:
		return false
	}
}
