package utils

import "testing"

func TestParsePixivWebRefreshTokenInputPlainToken(t *testing.T) {
	token, parsedCookie := ParsePixivWebRefreshTokenInput(" refresh-value ")
	if token != "refresh-value" || parsedCookie {
		t.Fatalf("ParsePixivWebRefreshTokenInput returned token=%q parsedCookie=%v", token, parsedCookie)
	}
}

func TestParsePixivWebRefreshTokenInputCookieWithRefreshToken(t *testing.T) {
	token, parsedCookie := ParsePixivWebRefreshTokenInput("foo=bar; refresh_token=refresh%2Fvalue; device_token=ignored")
	if token != "refresh/value" || !parsedCookie {
		t.Fatalf("ParsePixivWebRefreshTokenInput returned token=%q parsedCookie=%v", token, parsedCookie)
	}
}

func TestParsePixivWebRefreshTokenInputCookieWithoutRefreshToken(t *testing.T) {
	token, parsedCookie := ParsePixivWebRefreshTokenInput("PHPSESSID=abc; device_token=def; yuid_b=ghi")
	if token != "" || !parsedCookie {
		t.Fatalf("ParsePixivWebRefreshTokenInput returned token=%q parsedCookie=%v", token, parsedCookie)
	}
}

func TestParsePixivWebRefreshTokenInputSingleEqualsValueIsPlainToken(t *testing.T) {
	token, parsedCookie := ParsePixivWebRefreshTokenInput("opaque=refresh-token")
	if token != "opaque=refresh-token" || parsedCookie {
		t.Fatalf("ParsePixivWebRefreshTokenInput returned token=%q parsedCookie=%v", token, parsedCookie)
	}
}
