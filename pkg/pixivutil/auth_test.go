package pixivutil

import "testing"

func TestParseRefreshTokenInputPlainToken(t *testing.T) {
	token, parsedCookie := ParseRefreshTokenInput(" refresh-value ")
	if token != "refresh-value" || parsedCookie {
		t.Fatalf("ParseRefreshTokenInput returned token=%q parsedCookie=%v", token, parsedCookie)
	}
}

func TestParseRefreshTokenInputCookieWithRefreshToken(t *testing.T) {
	token, parsedCookie := ParseRefreshTokenInput("foo=bar; refresh_token=refresh%2Fvalue; device_token=ignored")
	if token != "refresh/value" || !parsedCookie {
		t.Fatalf("ParseRefreshTokenInput returned token=%q parsedCookie=%v", token, parsedCookie)
	}
}

func TestParseRefreshTokenInputCookieWithoutRefreshToken(t *testing.T) {
	token, parsedCookie := ParseRefreshTokenInput("PHPSESSID=abc; device_token=def; yuid_b=ghi")
	if token != "" || !parsedCookie {
		t.Fatalf("ParseRefreshTokenInput returned token=%q parsedCookie=%v", token, parsedCookie)
	}
}

func TestParseRefreshTokenInputSingleEqualsValueIsPlainToken(t *testing.T) {
	token, parsedCookie := ParseRefreshTokenInput("opaque=refresh-token")
	if token != "opaque=refresh-token" || parsedCookie {
		t.Fatalf("ParseRefreshTokenInput returned token=%q parsedCookie=%v", token, parsedCookie)
	}
}
