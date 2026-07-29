package loginhelper

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCallbackPrefersActiveLocalBridge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, err := writeCallbackEndpoint("http://127.0.0.1:41871/callback")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(path) })

	originalForward := forwardConfiguredCallbackForHandler
	forwardConfiguredCallbackForHandler = func(context.Context, string) (*RemoteCallbackSession, error) {
		t.Fatal("active local bridge must win over the remote relay")
		return nil, nil
	}
	t.Cleanup(func() { forwardConfiguredCallbackForHandler = originalForward })

	result, err := HandleCallback(context.Background(), "pixiv://account/login?code=local-code")
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:41871/callback#pixiv://account/login?code=local-code", result.LocalRelayURL)
}

func TestHandleCallbackForwardsOnlyAllowlistedRemoteCallback(t *testing.T) {
	originalRelay := callbackRelayURLForHandler
	originalForward := forwardConfiguredCallbackForHandler
	callbackRelayURLForHandler = func(string) (string, error) { return "", ErrNoActiveLocalCallback }
	var forwarded string
	forwardConfiguredCallbackForHandler = func(_ context.Context, rawURL string) (*RemoteCallbackSession, error) {
		forwarded = rawURL
		return &RemoteCallbackSession{ResultURL: "https://relay.example/result/test"}, nil
	}
	t.Cleanup(func() {
		callbackRelayURLForHandler = originalRelay
		forwardConfiguredCallbackForHandler = originalForward
	})

	result, err := HandleCallback(context.Background(), "pixiv://account/login?code=remote-code")
	require.NoError(t, err)
	assert.Empty(t, result.LocalRelayURL)
	require.NotNil(t, result.RemoteCallback)
	assert.Equal(t, "https://relay.example/result/test", result.RemoteCallback.ResultURL)
	assert.Equal(t, "pixiv://account/login?code=remote-code", forwarded)
}

func TestHandleCallbackDelegatesNonAllowlistedURL(t *testing.T) {
	originalDelegate := delegateToPreviousForHandler
	originalForward := forwardConfiguredCallbackForHandler
	var delegated string
	delegateToPreviousForHandler = func(_ context.Context, rawURL string) error {
		delegated = rawURL
		return nil
	}
	forwardConfiguredCallbackForHandler = func(context.Context, string) (*RemoteCallbackSession, error) {
		t.Fatal("non-allowlisted URL must never be sent to the remote relay")
		return nil, nil
	}
	t.Cleanup(func() {
		delegateToPreviousForHandler = originalDelegate
		forwardConfiguredCallbackForHandler = originalForward
	})

	const rawURL = "pixiv://account/other?code=must-not-forward"
	result, err := HandleCallback(context.Background(), rawURL)
	require.NoError(t, err)
	assert.Empty(t, result.LocalRelayURL)
	assert.Equal(t, rawURL, delegated)
}

func TestHandleCallbackDelegatesAllowlistedURLWhenRemoteRelayIsDisabled(t *testing.T) {
	originalRelay := callbackRelayURLForHandler
	originalForward := forwardConfiguredCallbackForHandler
	originalDelegate := delegateToPreviousForHandler
	callbackRelayURLForHandler = func(string) (string, error) { return "", ErrNoActiveLocalCallback }
	forwardConfiguredCallbackForHandler = func(context.Context, string) (*RemoteCallbackSession, error) { return nil, ErrNoConfiguredRelay }
	delegated := false
	delegateToPreviousForHandler = func(context.Context, string) error {
		delegated = true
		return nil
	}
	t.Cleanup(func() {
		callbackRelayURLForHandler = originalRelay
		forwardConfiguredCallbackForHandler = originalForward
		delegateToPreviousForHandler = originalDelegate
	})

	_, err := HandleCallback(context.Background(), "pixiv://account/login?code=delegate-code")
	require.NoError(t, err)
	assert.True(t, delegated)
}

func TestHandleCallbackPropagatesConfiguredRelayFailure(t *testing.T) {
	originalRelay := callbackRelayURLForHandler
	originalForward := forwardConfiguredCallbackForHandler
	callbackRelayURLForHandler = func(string) (string, error) { return "", ErrNoActiveLocalCallback }
	want := errors.New("relay rejected callback")
	forwardConfiguredCallbackForHandler = func(context.Context, string) (*RemoteCallbackSession, error) { return nil, want }
	t.Cleanup(func() {
		callbackRelayURLForHandler = originalRelay
		forwardConfiguredCallbackForHandler = originalForward
	})

	_, err := HandleCallback(context.Background(), "pixiv://account/login?code=failed-code")
	require.ErrorIs(t, err, want)
}
