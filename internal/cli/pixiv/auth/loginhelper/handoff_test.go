package loginhelper_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/auth/loginhelper"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

type trackingReadCloser struct {
	io.Reader
	closed atomic.Bool
}

func (body *trackingReadCloser) Close() error {
	body.closed.Store(true)
	return nil
}

type handoffRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper handoffRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func TestRemoteLoginHandoffDoesNotFollowCrossOriginRedirects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var received atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(target.Close)

			relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+"/received", status)
			}))
			t.Cleanup(relay.Close)

			_, err := loginhelper.StartRemoteLogin(context.Background(), loginhelper.RemoteLoginStart{
				Origin:    relay.URL,
				SessionID: "redirect-start",
				Proof:     "start-proof",
			})
			require.Error(t, err)
			require.Zero(t, received.Load(), "authorization handoff body must never be replayed to a redirect target")
		})
	}
}

func TestRemoteLoginCallbackDoesNotFollowCrossOriginRedirects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var received atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(target.Close)

			relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target.URL+"/received", status)
			}))
			t.Cleanup(relay.Close)

			require.NoError(t, loginhelper.SaveActiveRemoteLogin(loginhelper.ActiveRemoteLogin{
				Version:   1,
				Origin:    relay.URL,
				SessionID: "redirect-callback",
				Proof:     "callback-proof",
			}))
			_, err := loginhelper.ForwardActiveRemoteLoginCallback(context.Background(), "pixiv://account/login?code=one-time-code")
			require.Error(t, err)
			require.Zero(t, received.Load(), "Pixiv callback and proof must never be replayed to a redirect target")
		})
	}
}

func TestValidateAuthorizationURLAcceptsOnlyOfficialAppOAuthStart(t *testing.T) {
	session, err := pixiv.BeginLogin(pixiv.LoginOptions{})
	require.NoError(t, err)
	require.NoError(t, loginhelper.ValidateAuthorizationURL(session.AuthorizationURL()), "a URL produced by BeginLogin must remain valid")

	const valid = "https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=challenge-value&state=state-value"
	for _, raw := range []string{
		"http://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=challenge-value&state=state-value",
		"https://example.test/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=challenge-value&state=state-value",
		"https://app-api.pixiv.net:443/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=challenge-value&state=state-value",
		"https://user@app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=challenge-value&state=state-value",
		valid + "#fragment",
		"https://app-api.pixiv.net/web/v1/login?code_challenge_method=S256&code_challenge=challenge-value&state=state-value",
		"https://app-api.pixiv.net/web/v1/login?client=other-client&code_challenge_method=S256&code_challenge=challenge-value&state=state-value",
		"https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=plain&code_challenge=challenge-value&state=state-value",
		"https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&state=state-value",
		"https://app-api.pixiv.net/web/v1/login?client=pixiv-android&code_challenge_method=S256&code_challenge=challenge-value",
		valid + "&state=another-state",
	} {
		require.EqualError(t, loginhelper.ValidateAuthorizationURL(raw), "remote Pixiv login relay returned an invalid sign-in address", raw)
	}
}

func TestClearRemoteLoginHandoffPreservesNewerSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	old := loginhelper.RemoteLoginStart{Origin: "https://relay.example", SessionID: "old-session", Proof: "old-proof"}
	newer := loginhelper.ActiveRemoteLogin{Version: 1, Origin: "https://relay.example", SessionID: "new-session", Proof: "new-proof"}
	require.NoError(t, loginhelper.SaveActiveRemoteLogin(loginhelper.ActiveRemoteLogin{Version: 1, Origin: old.Origin, SessionID: old.SessionID, Proof: old.Proof}))
	require.NoError(t, loginhelper.SaveActiveRemoteLogin(newer))

	require.NoError(t, loginhelper.ClearRemoteLoginHandoff(old))
	active, err := loginhelper.LoadActiveRemoteLogin()
	require.NoError(t, err)
	require.Equal(t, newer, active)
}

func TestClearRemoteLoginHandoffReportsUnreadableState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	statePath, err := loginhelper.ActiveRemoteLoginPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(statePath, 0o700))

	err = loginhelper.ClearRemoteLoginHandoff(loginhelper.RemoteLoginStart{Origin: "https://relay.example", SessionID: "session", Proof: "proof"})
	require.EqualError(t, err, "could not clear active remote login handoff")
}

func TestForwardRemoteLoginCallbackFailsWhenLocalHandoffCannotBeConsumed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	active := loginhelper.ActiveRemoteLogin{Version: 1, Origin: "https://relay.example", SessionID: "session", Proof: "proof"}
	require.NoError(t, loginhelper.SaveActiveRemoteLogin(active))

	body := &trackingReadCloser{Reader: strings.NewReader("{}")}
	resultHeaders := make(http.Header)
	resultHeaders.Set(loginhelper.RelayResultURLHeader, "https://relay.example/result/YWJj")
	t.Cleanup(loginhelper.SetHandoffHTTPClient(&http.Client{Transport: handoffRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     resultHeaders,
			Body:       body,
		}, nil
	})}))
	t.Cleanup(loginhelper.SetClearActiveRemoteLoginForHandoff(func(loginhelper.ActiveRemoteLogin) error { return errors.New("local state is unavailable") }))

	result, err := loginhelper.ForwardActiveRemoteLoginCallback(context.Background(), "pixiv://account/login?code=one-time-code")
	require.Nil(t, result)
	require.EqualError(t, err, "could not clear active remote login handoff")
	require.True(t, body.closed.Load(), "a failed post-delivery cleanup must not leave the relay response open")
}
