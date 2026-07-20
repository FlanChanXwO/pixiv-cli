package cli

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteLoginFinalPageCentersAndHidesSensitiveFailure(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	writeLoginFinalPage(rec, true)
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(body, "text-align:center") || !strings.Contains(body, "display:flex") {
		t.Fatalf("success page not centered: %s", body)
	}
	if !strings.Contains(body, "登录成功") || !strings.Contains(body, "<h1>登录成功</h1>") {
		t.Fatalf("success title missing: %s", body)
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
	if !strings.Contains(body, "登录失败") {
		t.Fatalf("failure title missing: %s", body)
	}
	// 失败页不得回显敏感片段
	for _, secret := range []string{"token", "refresh", "Bearer", "code=", "password", "secret"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(secret)) {
			t.Fatalf("failure page leaked %q: %s", secret, body)
		}
	}
}
