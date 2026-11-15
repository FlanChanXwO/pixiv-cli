package oauth

import (
	"errors"
	"strings"
)

// errCookieRefreshTokenInput rejects web cookies without echoing credential input.
var errCookieRefreshTokenInput = errors.New("cookie input is not supported; provide a Pixiv App API refresh token")

// validateRefreshTokenInput accepts opaque App refresh tokens. A single x=y may
// be a token; known cookie names and multiple cookie pairs are rejected unchanged.
func validateRefreshTokenInput(input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if looksLikeCookie(value) {
		return "", errCookieRefreshTokenInput
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
