package fanbox_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestSessionIdentity(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "FANBOXSESSID=session-fixture" || r.Header.Get("Referer") != "https://www.fanbox.cc/" {
			t.Error("incorrect session headers")
		}
		fmt.Fprint(w, `<meta name="metadata" content='{"context":{"user":{"userId":"21","name":"fixture","creatorId":"artist","creatorStatus":"active"}}}'>`)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	user, err := client.CurrentUser(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(21), user.UserID)
	require.Equal(t, "fixture", user.DisplayName)
	require.Equal(t, "artist", user.CreatorID)
	require.True(t, user.IsCreator)
	require.NoError(t, client.ValidateSession(t.Context()))
}

func TestSessionFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		reason     sdk.Reason
	}{
		{"expired", "", http.StatusUnauthorized, sdk.CredentialsExpired},
		{"forbidden", `{}`, http.StatusForbidden, sdk.Forbidden},
		{"metadata missing", `<html></html>`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"metadata invalid", `<meta name="metadata" content='secret-fixture'>`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"user missing", `<meta name="metadata" content='{"context":{}}'>`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"invalid id", `<meta name="metadata" content='{"context":{"user":{"userId":0,"name":"fixture"}}}'>`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"empty name", `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":" "}}}'>`, http.StatusOK, sdk.MalformedUpstreamResponse},
		{"upstream", "", http.StatusServiceUnavailable, sdk.UpstreamUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			err = client.ValidateSession(t.Context())
			require.Equal(t, tc.reason, sdk.ReasonOf(err))
			require.NotContains(t, fmt.Sprint(err), "secret-fixture")
		})
	}
}

func TestSessionCreatorStatus(t *testing.T) {
	for _, tc := range []struct {
		status, explicit, expected string
		isCreator                  bool
	}{
		{"false", "true", "true", true},
		{"true", "false", "false", false},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintf(w, `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":"fixture","creatorStatus":%s,"isCreator":%s}}}'>`, tc.status, tc.explicit)
			}))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
			require.NoError(t, err)
			user, err := client.CurrentUser(t.Context())
			require.NoError(t, err)
			require.Equal(t, tc.expected, user.CreatorStatus)
			require.Equal(t, tc.isCreator, user.IsCreator)
		})
	}
}

func TestSessionCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- client.ValidateSession(ctx) }()
	<-started
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	require.NoError(t, client.Close(t.Context()))
}

type recoveryFailure struct {
	name, solverBody    string
	challenge, disabled bool
	reason              sdk.Reason
	solves, requests    int32
}

func TestChallengeFailures(t *testing.T) {
	for _, tc := range []recoveryFailure{
		{"ordinary forbidden", ``, false, false, sdk.Forbidden, 0, 1},
		{"disabled", ``, true, true, sdk.ChallengeRequired, 0, 1},
		{"repeated challenge", `{"status":"ok","solution":{"userAgent":"solved-agent","cookies":[{"name":"cf_clearance","value":"clearance-fixture"}]}}`, true, false, sdk.ChallengeRequired, 1, 2},
		{"solver failure", `{"status":"error","message":"secret-fixture"}`, true, false, sdk.ChallengeRequired, 1, 1},
		{"malformed solver", `{`, true, false, sdk.MalformedUpstreamResponse, 1, 1},
		{"missing clearance", `{"status":"ok","solution":{"userAgent":"fixture","cookies":[]}}`, true, false, sdk.MalformedUpstreamResponse, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var solves, calls atomic.Int32
			solver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { solves.Add(1); fmt.Fprint(w, tc.solverBody) }))
			defer solver.Close()
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if tc.challenge {
					w.Header().Set("Cf-Mitigated", "challenge")
				}
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"message":"business challenge word"}`)
			}))
			defer server.Close()
			httpClient := server.Client()
			routeToServer(httpClient, server)
			endpoint := solver.URL
			if tc.disabled {
				endpoint = ""
			}
			client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient, FlareSolverrURL: endpoint})
			require.NoError(t, err)
			err = client.ValidateSession(t.Context())
			require.Equal(t, tc.reason, sdk.ReasonOf(err))
			require.NotContains(t, fmt.Sprint(err), "secret-fixture")
			require.Equal(t, tc.solves, solves.Load())
			require.Equal(t, tc.requests, calls.Load())
			require.NoError(t, client.Close(t.Context()))
		})
	}
}
