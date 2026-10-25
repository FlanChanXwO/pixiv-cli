package loginrelay_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/loginrelay"
)

func TestPagesRenderEscapedLoginContent(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := loginrelay.WriteManual(&out, "https://app-api.pixiv.net/web/v1/login?state=one&code_challenge=two")
	if err != nil {
		t.Fatalf("loginrelay.WriteManual() error = %v", err)
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
	if err := loginrelay.WriteCallbackRelay(&out); err != nil {
		t.Fatalf("loginrelay.WriteCallbackRelay() error = %v", err)
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

	success := httptest.NewRecorder()
	loginrelay.WriteFinalPage(success, true)
	if !strings.Contains(success.Body.String(), "Login successful") {
		t.Fatalf("success page missing heading: %s", success.Body.String())
	}

	failure := httptest.NewRecorder()
	loginrelay.WriteFinalPage(failure, false)
	body := strings.ToLower(failure.Body.String())
	if !strings.Contains(body, "login failed") {
		t.Fatalf("failure page missing heading: %s", body)
	}
	if failure.Code != http.StatusBadRequest {
		t.Fatalf("failure status = %d, want 400", failure.Code)
	}
	for _, forbidden := range []string{"token", "refresh", "bearer", "code=", "password", "secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("failure page leaked %q: %s", forbidden, body)
		}
	}
}
