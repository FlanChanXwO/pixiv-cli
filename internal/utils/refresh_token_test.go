package utils

import (
	"errors"
	"testing"
)

func TestValidateRefreshTokenInputAcceptsOpaqueToken(t *testing.T) {
	token, err := ValidateRefreshTokenInput(" opaque=value ")
	if err != nil {
		t.Fatalf("ValidateRefreshTokenInput() error = %v", err)
	}
	if token != "opaque=value" {
		t.Fatalf("token = %q", token)
	}
}

func TestValidateRefreshTokenInputRejectsCookie(t *testing.T) {
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
