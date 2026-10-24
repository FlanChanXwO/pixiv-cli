package fanboxapi_test

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/fanboxapi"
)

func TestNormalizeCookieHeader(t *testing.T) {
	normalized, err := fanboxapi.NormalizeCookieHeader("  FANBOXSESSID = abc123  ; other=def  ")
	if err != nil || normalized != "FANBOXSESSID=abc123; other=def" {
		t.Fatalf("normalized = %q err=%v", normalized, err)
	}
	for _, header := range []string{
		"",
		"other=value",
		"FANBOXSESSID=abc\ninjected=1",
		"FANBOXSESSID",
		"FANBOXSESSID=abc; FANBOXSESSID=def",
		"FANBOXSESSID=",
		"FANBOXSESSID=abc; bad name=value",
	} {
		if _, err := fanboxapi.NormalizeCookieHeader(header); err == nil {
			t.Fatalf("NormalizeCookieHeader(%q) unexpectedly succeeded", header)
		}
	}
	_, err = fanboxapi.NormalizeCookieHeader("FANBOXSESSID=super-secret-canary; broken pair")
	if err == nil || strings.Contains(err.Error(), "super-secret-canary") {
		t.Fatalf("malformed cookie error leaked value: %v", err)
	}
}
