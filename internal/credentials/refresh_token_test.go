package credentials

import (
	"errors"
	"testing"
)

func TestValidateRefreshTokenInputRejectsCookie(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"refresh_token=secret",
		"PHPSESSID=secret",
		"session=secret",
		"csrftoken=secret",
		"opaque=one; another=two",
		"Cookie: opaque=one",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := ValidateRefreshTokenInput(input)
			if !errors.Is(err, ErrCookieRefreshTokenInput) {
				t.Fatalf("ValidateRefreshTokenInput(%q) error = %v", input, err)
			}
		})
	}
}

func TestValidateRefreshTokenInputAcceptsOpaqueToken(t *testing.T) {
	t.Parallel()

	token, err := ValidateRefreshTokenInput(" opaque=value ")
	if err != nil {
		t.Fatalf("ValidateRefreshTokenInput() error = %v", err)
	}
	if token != "opaque=value" {
		t.Fatalf("token = %q", token)
	}
}
