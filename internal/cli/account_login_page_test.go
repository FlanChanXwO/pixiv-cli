package cli

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteLoginFinalPageCentersAndHidesSensitiveFailure(t *testing.T) {
	t.Parallel()
	form := httptest.NewRecorder()
	writeLoginForm(form, "https://app-api.pixiv.net/web/v1/login?state=test")
	if !strings.Contains(form.Body.String(), "<title>pixiv-cli</title>") {
		t.Fatalf("manual page title = %s", form.Body.String())
	}

	callback := httptest.NewRecorder()
	writeLoginCallbackRelayPage(callback)
	if !strings.Contains(callback.Body.String(), "<title>pixiv-cli</title>") || strings.Contains(callback.Body.String(), "document.title = \"Login failed\"") {
		t.Fatalf("callback page must retain pixiv-cli title: %s", callback.Body.String())
	}

	rec := httptest.NewRecorder()
	writeLoginFinalPage(rec, true)
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(body, "text-align:center") || !strings.Contains(body, "display:flex") {
		t.Fatalf("success page not centered: %s", body)
	}
	if !strings.Contains(body, "Login successful") || !strings.Contains(body, "<h1>Login successful</h1>") || !strings.Contains(body, `<html lang="en">`) || !strings.Contains(body, "<title>pixiv-cli</title>") {
		t.Fatalf("success title missing: %s", body)
	}
	if strings.Contains(body, "登录") {
		t.Fatalf("success page contains non-English fixed text: %s", body)
	}

	rec = httptest.NewRecorder()
	writeLoginFinalPage(rec, false)
	body = rec.Body.String()
	if rec.Code != 400 {
		t.Fatalf("fail status=%d", rec.Code)
	}
	if !strings.Contains(body, "text-align:center") || !strings.Contains(body, "display:flex") {
		t.Fatalf("failure page not centered: %s", body)
	}
	if !strings.Contains(body, "Login failed") || !strings.Contains(body, "<title>pixiv-cli</title>") {
		t.Fatalf("failure title missing: %s", body)
	}
	if strings.Contains(body, "登录") {
		t.Fatalf("failure page contains non-English fixed text: %s", body)
	}
	// 失败页不得回显敏感片段
	for _, secret := range []string{"token", "refresh", "Bearer", "code=", "password", "secret"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(secret)) {
			t.Fatalf("failure page leaked %q: %s", secret, body)
		}
	}
}
