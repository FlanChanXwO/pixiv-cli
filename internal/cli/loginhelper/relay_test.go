package loginhelper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardConfiguredCallbackPreflightsAndPostsOnlyWhitelistedURL(t *testing.T) {
	const secret = "relay-client-secret-canary"
	const callback = "pixiv://account/login?code=callback-code"
	var requests int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer "+secret, r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/session":
			require.Equal(t, http.MethodGet, r.Method)
			requests++
			w.WriteHeader(http.StatusNoContent)
		case "/callback":
			require.Equal(t, http.MethodPost, r.Method)
			requests++
			w.Header().Set(RelayResultURLHeader, server.URL+"/result/AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
			w.WriteHeader(http.StatusOK)
			require.NoError(t, json.NewEncoder(w).Encode(relayCallbackCompletion{Success: true}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "config.toml")
	restorePath := config.SetFilePathForTest(path)
	defer restorePath()
	require.NoError(t, config.WritePrivateFile(path, []byte("[login]\nrelay_target_url = \""+server.URL+"\"\nrelay_secret = \""+secret+"\"\n")))

	session, err := ForwardConfiguredCallback(context.Background(), callback)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.NoError(t, session.Complete())
	assert.Equal(t, 2, requests)
	_, err = ForwardConfiguredCallback(context.Background(), "pixiv://other/path?code=not-allowed")
	require.Error(t, err)
	assert.Equal(t, 2, requests)
}

func TestRelayEndpointURLRejectsCredentialBearingURL(t *testing.T) {
	_, err := relayEndpointURL("https://user:secret@example.invalid", relayCallbackPath)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")
}
