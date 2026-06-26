package pixiv

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshAndRetryOnAuthError(t *testing.T) {
	var refreshed bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/illust/detail" {
			t.Fatalf("unexpected api path %s", r.URL.Path)
		}
		if !refreshed {
			http.Error(w, `{"error":{"message":"unauthorized"}}`, http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(IllustDetail{Illust: Illust{ID: 42, Title: "ok"}})
	}))
	defer api.Close()

	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse refresh form: %v", err)
		}
		if got := r.Form.Get("client_id"); got != DefaultOAuthClientID {
			t.Fatalf("client_id = %q, want %q", got, DefaultOAuthClientID)
		}
		if got := r.Form.Get("client_secret"); got != DefaultOAuthClientSecret {
			t.Fatalf("client_secret = %q, want %q", got, DefaultOAuthClientSecret)
		}
		refreshed = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"user":          map[string]any{"id": "7"},
		})
	}))
	defer oauth.Close()

	client := New("old-refresh", WithBaseURLs(api.URL, oauth.URL))
	result, err := client.IllustDetail(context.Background(), 42)
	if err != nil {
		t.Fatalf("IllustDetail returned error: %v", err)
	}
	if result.Illust.ID != 42 || client.UserID() != 7 || client.RefreshTokenValue() != "new-refresh" {
		t.Fatalf("unexpected result/client state: result=%+v user=%d refresh=%q", result, client.UserID(), client.RefreshTokenValue())
	}
}

func TestClientExtractsRefreshTokenFromCookieInput(t *testing.T) {
	client := New("PHPSESSID=web; refresh_token=initial%2Frefresh")
	if client.RefreshTokenValue() != "initial/refresh" {
		t.Fatalf("RefreshTokenValue = %q", client.RefreshTokenValue())
	}
	client.SetRefreshToken("device_token=device; refresh_token=updated")
	if client.RefreshTokenValue() != "updated" {
		t.Fatalf("RefreshTokenValue after SetRefreshToken = %q", client.RefreshTokenValue())
	}
}
