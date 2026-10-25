package oauth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	oauth "github.com/FlanChanXwO/pixiv-cli/internal/pixivapi/oauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthTokenLifecycle(t *testing.T) {
	t.Run("refresh keeps identity state", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
			assert.Equal(t, "old/refresh", r.Form.Get("refresh_token"))
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "refresh_token": "new-refresh", "user": map[string]any{"id": "7", "name": "alice"}})
		}))
		defer server.Close()
		client := oauth.New("old/refresh", oauth.Config{HTTPClient: server.Client(), BaseURL: server.URL})
		require.NoError(t, client.Refresh(context.Background()))
		assert.Equal(t, "access", client.AccessToken())
		assert.Equal(t, "new-refresh", client.RefreshTokenValue())
		assert.Equal(t, int64(7), client.UserID())
		assert.Equal(t, "alice", client.UserName())
	})

	t.Run("exchange stores token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
			assert.Equal(t, "code", r.Form.Get("code"))
			assert.Equal(t, "verifier", r.Form.Get("code_verifier"))
			_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"access_token": "access", "refresh_token": "refresh", "user": map[string]any{"id": 9, "name": "bob"}}})
		}))
		defer server.Close()
		client := oauth.New("", oauth.Config{HTTPClient: server.Client(), BaseURL: server.URL})
		token, err := client.ExchangeAuthorizationCode(context.Background(), "code", "verifier")
		require.NoError(t, err)
		assert.Equal(t, "refresh", token.RefreshToken)
		assert.Equal(t, int64(9), token.UserID)
		assert.Equal(t, "access", client.AccessToken())
	})
}

func TestOAuthRefreshRejectsInvalidInput(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := oauth.New("refresh-token", oauth.Config{BaseURL: "https://example.invalid"}).Refresh(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled refresh = %v", err)
	}
	client := oauth.New("PHPSESSID=secret; refresh_token=another", oauth.Config{})
	require.ErrorContains(t, client.Refresh(context.Background()), "cookie input is not supported; provide a Pixiv App API refresh token")
	require.ErrorContains(t, oauth.New("csrftoken=secret", oauth.Config{}).Refresh(context.Background()), "cookie input is not supported; provide a Pixiv App API refresh token")
}
