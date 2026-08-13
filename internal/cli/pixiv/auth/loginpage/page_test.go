package loginpage_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/auth/loginpage"
)

func TestWriteManualEscapesLoginURL(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := loginpage.WriteManual(&out, "https://app-api.pixiv.net/web/v1/login?state=one&code_challenge=two")
	if err != nil {
		t.Fatalf("WriteManual() error = %v", err)
	}
	body := out.String()
	if !strings.Contains(body, `<title>pixiv-cli</title>`) || !strings.Contains(body, `action="/manual"`) {
		t.Fatalf("manual page structure missing: %s", body)
	}
	if !strings.Contains(body, `state=one&amp;code_challenge=two`) {
		t.Fatalf("manual page did not escape URL query: %s", body)
	}
}

func TestWriteCallbackRelayContainsLocalPOSTBridge(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := loginpage.WriteCallbackRelay(&out); err != nil {
		t.Fatalf("WriteCallbackRelay() error = %v", err)
	}
	body := out.String()
	for _, want := range []string{
		`window.location.hash.slice(1)`,
		`window.history.replaceState`,
		`form.action = "/manual"`,
		`input.value = completionURL`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("callback relay page missing %q: %s", want, body)
		}
	}
}

func TestWriteResultKeepsFailureGeneric(t *testing.T) {
	t.Parallel()

	var success bytes.Buffer
	if err := loginpage.WriteResult(&success, true); err != nil {
		t.Fatalf("WriteResult(success) error = %v", err)
	}
	if !strings.Contains(success.String(), "Login successful") {
		t.Fatalf("success page missing heading: %s", success.String())
	}

	var failure bytes.Buffer
	if err := loginpage.WriteResult(&failure, false); err != nil {
		t.Fatalf("WriteResult(failure) error = %v", err)
	}
	body := strings.ToLower(failure.String())
	if !strings.Contains(body, "login failed") {
		t.Fatalf("failure page missing heading: %s", body)
	}
	for _, forbidden := range []string{"token", "refresh", "bearer", "code=", "password", "secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("failure page leaked %q: %s", forbidden, body)
		}
	}
}

func TestPagesDoNotOverrideBrowserFavicon(t *testing.T) {
	t.Parallel()

	var manual, callback, success, failure bytes.Buffer
	if err := loginpage.WriteManual(&manual, "https://app-api.pixiv.net/web/v1/login?state=test"); err != nil {
		t.Fatalf("WriteManual() error = %v", err)
	}
	if err := loginpage.WriteCallbackRelay(&callback); err != nil {
		t.Fatalf("WriteCallbackRelay() error = %v", err)
	}
	if err := loginpage.WriteResult(&success, true); err != nil {
		t.Fatalf("WriteResult(success) error = %v", err)
	}
	if err := loginpage.WriteResult(&failure, false); err != nil {
		t.Fatalf("WriteResult(failure) error = %v", err)
	}
	for _, page := range []string{manual.String(), callback.String(), success.String(), failure.String()} {
		if strings.Contains(strings.ToLower(page), `rel="icon"`) {
			t.Fatalf("login page overrides favicon: %s", page)
		}
	}
}

func TestPagesUseSingleEnglishLocale(t *testing.T) {
	t.Parallel()

	var manual, callback, success, failure bytes.Buffer
	if err := loginpage.WriteManual(&manual, "https://app-api.pixiv.net/web/v1/login?state=test"); err != nil {
		t.Fatalf("WriteManual() error = %v", err)
	}
	if err := loginpage.WriteCallbackRelay(&callback); err != nil {
		t.Fatalf("WriteCallbackRelay() error = %v", err)
	}
	if err := loginpage.WriteResult(&success, true); err != nil {
		t.Fatalf("WriteResult(success) error = %v", err)
	}
	if err := loginpage.WriteResult(&failure, false); err != nil {
		t.Fatalf("WriteResult(failure) error = %v", err)
	}
	for _, page := range []string{manual.String(), callback.String(), success.String(), failure.String()} {
		for _, forbidden := range []string{"data-i18n", "navigator.language", "完成登录", "ログイン"} {
			if strings.Contains(page, forbidden) {
				t.Fatalf("login page retained locale switching content %q: %s", forbidden, page)
			}
		}
	}
}
