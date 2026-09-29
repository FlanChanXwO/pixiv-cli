package auth_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"testing/synctest"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/auth"
	"github.com/stretchr/testify/require"
)

func TestBearerChecksEachRequestAndCanonicalChallenge(t *testing.T) {
	f := newAuthorizationFixture(t)
	pair := tokenRequest(t, f, codeForm(t, f), 200)
	h, err := auth.NewHandler("http://instance.test/prefix", f.store)
	require.NoError(t, err)
	calls := 0
	protected := h.RequireBearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	for _, token := range []string{"", "unknown", f.secret} {
		request := httptest.NewRequest("POST", "http://untrusted.test/prefix/mcp", nil)
		request.Header.Set("X-Forwarded-Host", "untrusted.test")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		protected.ServeHTTP(response, request)
		require.Equal(t, 401, response.Code)
		require.Contains(t, response.Header().Get("WWW-Authenticate"), `resource_metadata="http://instance.test/.well-known/oauth-protected-resource/prefix/mcp"`)
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	}
	require.Zero(t, calls)
	request := httptest.NewRequest("POST", "http://instance.test/prefix/mcp", nil)
	request.Header.Set("Authorization", "Bearer "+pair["access_token"].(string))
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	require.Equal(t, 204, response.Code)
	require.Equal(t, 1, calls)
	_, err = f.store.Init(t.Context(), true)
	require.NoError(t, err)
	response = httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	require.Equal(t, 401, response.Code)
	require.Equal(t, 1, calls)
}

func bearerRequest(h http.Handler, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("POST", "/prefix/mcp", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestBearerDeadlineDoesNotCancelAuthorizedRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h, _, form := directCode(t)
		pair := decodedToken(t, directToken(h, form))
		token := pair["access_token"].(string)
		release := make(chan struct{})
		done := make(chan *httptest.ResponseRecorder, 1)
		protected := h.RequireBearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-release
			if r.Context().Err() != nil {
				w.WriteHeader(500)
				return
			}
			w.WriteHeader(204)
		}))
		go func() { done <- bearerRequest(protected, token) }()
		synctest.Wait()
		time.Sleep(time.Hour - time.Nanosecond)
		fast := h.RequireBearer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
		require.Equal(t, 204, bearerRequest(fast, token).Code)
		time.Sleep(time.Nanosecond)
		require.Equal(t, 401, bearerRequest(fast, token).Code)
		close(release)
		require.Equal(t, 204, (<-done).Code)
	})
}

func TestBearerReplayRevokesAccessAndRejectsOtherResources(t *testing.T) {
	h, store, form := directCode(t)
	pair := decodedToken(t, directToken(h, form))
	token := pair["access_token"].(string)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	protected := h.RequireBearer(next)
	other, err := auth.NewHandler("http://other.test/prefix", store)
	require.NoError(t, err)
	require.Equal(t, 401, bearerRequest(other.RequireBearer(next), token).Code)
	require.Equal(t, 401, bearerRequest(protected, pair["refresh_token"].(string)).Code)
	refresh := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {pair["refresh_token"].(string)}, "client_id": {form.Get("client_id")}, "resource": {form.Get("resource")}}
	rotated := decodedToken(t, directToken(h, refresh))
	require.Equal(t, 204, bearerRequest(protected, token).Code)
	require.Equal(t, 400, directToken(h, refresh).Code)
	require.Equal(t, 401, bearerRequest(protected, token).Code)
	require.Equal(t, 401, bearerRequest(protected, rotated["access_token"].(string)).Code)
}

func TestBearerRejectsAmbiguousAuthorizationHeaders(t *testing.T) {
	h, _, form := directCode(t)
	pair := decodedToken(t, directToken(h, form))
	protected := h.RequireBearer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	request := httptest.NewRequest("POST", "/prefix/mcp", nil)
	request.Header.Add("Authorization", "Bearer "+pair["access_token"].(string))
	request.Header.Add("Authorization", "Bearer unknown")
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	require.Equal(t, 401, response.Code)
	require.NotEmpty(t, response.Header().Get("WWW-Authenticate"))
}
