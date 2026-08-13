package protocol_test

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
)

func TestNormalizeCookieHeader(t *testing.T) {
	normalized, err := protocol.NormalizeCookieHeader("  FANBOXSESSID = abc123  ; other=def  ")
	if err != nil {
		t.Fatalf("NormalizeCookieHeader() error = %v", err)
	}
	if got, want := normalized, "FANBOXSESSID=abc123; other=def"; got != want {
		t.Fatalf("normalized = %q, want %q", got, want)
	}
}

func TestNormalizeCookieHeaderRejectsMalformed(t *testing.T) {
	for _, header := range []string{
		"",
		"other=value",
		"FANBOXSESSID=abc\ninjected=1",
		"FANBOXSESSID",
		"FANBOXSESSID=abc; FANBOXSESSID=def",
		"FANBOXSESSID=",
		"FANBOXSESSID=abc; bad name=value",
	} {
		if _, err := protocol.NormalizeCookieHeader(header); err == nil {
			t.Fatalf("NormalizeCookieHeader(%q) unexpectedly succeeded", header)
		}
	}
}

func TestNormalizeCookieHeaderErrorNeverEchoesValue(t *testing.T) {
	_, err := protocol.NormalizeCookieHeader("FANBOXSESSID=super-secret-canary; broken pair")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "super-secret-canary") {
		t.Fatalf("error disclosed cookie value: %v", err)
	}
	if got := protocol.RedactCookieHeader("FANBOXSESSID=super-secret-canary"); strings.Contains(got, "super-secret-canary") {
		t.Fatalf("RedactCookieHeader disclosed value: %q", got)
	}
}
