package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshOwnsIdentityState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		assert.Equal(t, "old/refresh", r.Form.Get("refresh_token"))
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "refresh_token": "new-refresh", "user": map[string]any{"id": "7", "name": "alice"}})
	}))
	defer server.Close()
	client := New("PHPSESSID=x; refresh_token=old%2Frefresh", WithHTTPClient(server.Client()), WithBaseURL(server.URL))
	require.NoError(t, client.Refresh(context.Background()))
	assert.Equal(t, "access", client.AccessToken())
	assert.Equal(t, "new-refresh", client.RefreshTokenValue())
	assert.Equal(t, int64(7), client.UserID())
	assert.Equal(t, "alice", client.UserName())
}

func TestExchangeAuthorizationCodeStoresToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
		assert.Equal(t, "code", r.Form.Get("code"))
		assert.Equal(t, "verifier", r.Form.Get("code_verifier"))
		_ = json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"access_token": "access", "refresh_token": "refresh", "user": map[string]any{"id": 9, "name": "bob"}}})
	}))
	defer server.Close()
	client := New("", WithHTTPClient(server.Client()), WithBaseURL(server.URL))
	token, err := client.ExchangeAuthorizationCode(context.Background(), "code", "verifier")
	require.NoError(t, err)
	assert.Equal(t, "refresh", token.RefreshToken)
	assert.Equal(t, int64(9), token.UserID)
	assert.True(t, client.IsAuthenticated())
}
