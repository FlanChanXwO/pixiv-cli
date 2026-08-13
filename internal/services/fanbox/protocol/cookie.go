package protocol

import (
	"errors"
	"strings"
)

// NormalizeCookieHeader 校验用户提供的完整 Cookie header，并规范化 pair 两侧空白。
// 它只接受包含非空 FANBOXSESSID 的标准 name=value pair；错误绝不回显 Cookie 内容。
func NormalizeCookieHeader(header string) (string, error) {
	if strings.TrimSpace(header) == "" {
		return "", errors.New("FANBOX cookie header is required")
	}
	if strings.ContainsAny(header, "\r\n") {
		return "", errors.New("FANBOX cookie header must not contain line breaks")
	}

	pairs := strings.Split(header, ";")
	normalized := make([]string, 0, len(pairs))
	seen := make(map[string]struct{}, len(pairs))
	hasSessionID := false
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		name, value, ok := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if !ok || name == "" || !validCookieName(name) || value == "" || !validCookieValue(value) {
			return "", errors.New("FANBOX cookie header contains a malformed cookie pair")
		}
		if _, exists := seen[name]; exists {
			return "", errors.New("FANBOX cookie header contains duplicate cookie names")
		}
		seen[name] = struct{}{}
		if name == "FANBOXSESSID" {
			hasSessionID = true
		}
		normalized = append(normalized, name+"="+value)
	}
	if !hasSessionID {
		return "", errors.New("FANBOX cookie header must contain FANBOXSESSID")
	}
	return strings.Join(normalized, "; "), nil
}

// RedactCookieHeader 返回可安全展示的固定摘要，永远不包含原始 Cookie 内容。
func RedactCookieHeader(_ string) string {
	return "FANBOX cookie [REDACTED]"
}

func validCookieName(name string) bool {
	for _, char := range name {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", char)) {
			return false
		}
	}
	return true
}

func validCookieValue(value string) bool {
	for _, char := range value {
		// RFC 6265 cookie-octet 排除控制符、空白、双引号、逗号、分号和反斜杠。
		if char < 0x21 || char > 0x7e || strings.ContainsRune("\",;\\", char) {
			return false
		}
	}
	return true
}
