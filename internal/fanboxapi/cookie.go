package fanboxapi

import (
	"errors"
	"strings"
)

// sessionCookie 只接受一个 FANBOXSESSID 值，拒绝额外的 Cookie 对。
func sessionCookie(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n") {
		return "", errors.New("FANBOX session value must not contain line breaks")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("FANBOX session value is required")
	}
	if !validCookieValue(value) {
		return "", errors.New("FANBOX session value contains an invalid cookie character")
	}
	return "FANBOXSESSID=" + value, nil
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
