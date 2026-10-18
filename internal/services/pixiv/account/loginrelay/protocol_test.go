package loginrelay_test

import (
	"encoding/json"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account/loginrelay"
)

func TestCallbackURLAllowlist(t *testing.T) {
	for _, tt := range []struct {
		url     string
		allowed bool
	}{
		{"pixiv://account/login?code=one-time-code", true},
		{"  PIXIV://ACCOUNT/login?code=one-time-code  ", true},
		{"pixiv://account/login?code=one-time-code&state=session-state", true},
		{"https://account/login?code=one-time-code", false},
		{"pixiv://other/login?code=one-time-code", false},
		{"pixiv://account/remote-login?code=one-time-code", false},
		{"pixiv://account/login", false},
		{"pixiv://account/login?code=%20", false},
		{"%", false},
	} {
		if got := loginrelay.IsAllowedPixivCallbackURL(tt.url); got != tt.allowed {
			t.Errorf("IsAllowedPixivCallbackURL(%q) = %v, want %v", tt.url, got, tt.allowed)
		}
	}
}

func TestRemoteLoginWireContract(t *testing.T) {
	body, err := json.Marshal(loginrelay.RemoteLoginStartResponse{AuthorizationURL: "https://app-api.pixiv.net/web/v1/login"})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"authorization_url":"https://app-api.pixiv.net/web/v1/login"}` {
		t.Fatalf("unexpected start response: %s", body)
	}
	if loginrelay.RelayResultURLHeader != "X-Pixiv-Relay-Result-URL" {
		t.Fatal("result header changed")
	}
}
