package loginhelper_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth/loginhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCallbackPrefersActiveLocalBridge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := loginhelper.WriteCallbackEndpoint("http://127.0.0.1:41871/callback")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path) })

	t.Cleanup(loginhelper.SetForwardActiveRemoteCallbackForHandler(func(context.Context, string) (*loginhelper.RemoteCallbackSession, error) {
		t.Fatal("active local bridge must win over the remote handoff")
		return nil, nil
	}))

	result, err := loginhelper.HandleCallback(context.Background(), "pixiv://account/login?code=local-code")
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:41871/callback#pixiv://account/login?code=local-code", result.LocalRelayURL)
}

func TestHandleCallbackParsesExactRemoteLoginStart(t *testing.T) {
	raw := "pixiv://account/remote-login?origin=https%3A%2F%2Frelay.example%2Fpixiv&session=session-id&access=proof-id"
	result, err := loginhelper.HandleCallback(context.Background(), raw)
	require.NoError(t, err)
	require.Equal(t, &loginhelper.RemoteLoginStart{Origin: "https://relay.example/pixiv", SessionID: "session-id", Proof: "proof-id"}, result.RemoteLoginStart)
}

func TestHandleCallbackRejectsUnsafeRemoteLoginStart(t *testing.T) {
	for _, raw := range []string{
		"pixiv://account/remote-login?origin=https%3A%2F%2Frelay.example&session=session-id",
		"pixiv://account/remote-login?origin=https%3A%2F%2Frelay.example&session=session-id&access=proof-id&next=https%3A%2F%2Fexample.test",
		"pixiv://account/remote-login?origin=https%3A%2F%2Fuser%3Asecret%40relay.example&session=session-id&access=proof-id",
	} {
		_, err := loginhelper.HandleCallback(context.Background(), raw)
		require.EqualError(t, err, "invalid remote login start link")
	}
}

func TestHandleCallbackDelegatesWithoutActiveHandoff(t *testing.T) {
	t.Cleanup(loginhelper.SetCallbackRelayURLForHandler(func(string) (string, error) { return "", loginhelper.ErrNoActiveLocalCallback }))
	t.Cleanup(loginhelper.SetForwardActiveRemoteCallbackForHandler(func(context.Context, string) (*loginhelper.RemoteCallbackSession, error) {
		return nil, loginhelper.ErrNoActiveRemoteLogin
	}))
	delegated := ""
	t.Cleanup(loginhelper.SetDelegateToPreviousForHandler(func(_ context.Context, raw string) error {
		delegated = raw
		return nil
	}))

	const callback = "pixiv://account/login?code=delegate-code"
	result, err := loginhelper.HandleCallback(context.Background(), callback)
	require.NoError(t, err)
	assert.Empty(t, result.LocalRelayURL)
	assert.Nil(t, result.RemoteCallback)
	assert.Equal(t, callback, delegated)
}

func TestHandleCallbackDelegatesAfterRemoteHandoffIsCleared(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	start := loginhelper.RemoteLoginStart{Origin: "https://relay.example", SessionID: "failed-session", Proof: "failed-proof"}
	require.NoError(t, loginhelper.SaveActiveRemoteLogin(loginhelper.ActiveRemoteLogin{Version: 1, Origin: start.Origin, SessionID: start.SessionID, Proof: start.Proof}))
	require.NoError(t, loginhelper.ClearRemoteLoginHandoff(start))

	t.Cleanup(loginhelper.SetCallbackRelayURLForHandler(func(string) (string, error) { return "", loginhelper.ErrNoActiveLocalCallback }))
	delegated := ""
	t.Cleanup(loginhelper.SetDelegateToPreviousForHandler(func(_ context.Context, raw string) error {
		delegated = raw
		return nil
	}))

	const callback = "pixiv://account/login?code=delegate-after-failure"
	_, err := loginhelper.HandleCallback(context.Background(), callback)
	require.NoError(t, err)
	assert.Equal(t, callback, delegated)
}

func TestHandleCallbackPropagatesActiveHandoffFailure(t *testing.T) {
	t.Cleanup(loginhelper.SetCallbackRelayURLForHandler(func(string) (string, error) { return "", loginhelper.ErrNoActiveLocalCallback }))
	want := errors.New("relay rejected callback")
	t.Cleanup(loginhelper.SetForwardActiveRemoteCallbackForHandler(func(context.Context, string) (*loginhelper.RemoteCallbackSession, error) { return nil, want }))

	_, err := loginhelper.HandleCallback(context.Background(), "pixiv://account/login?code=failed-code")
	require.ErrorIs(t, err, want)
}
