package fanbox_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
)

func TestChallengeRecovery(t *testing.T) {
	var solves atomic.Int32
	solver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if json.NewDecoder(r.Body).Decode(&payload) != nil {
			t.Error("invalid control request")
		}
		if r.URL.Path != "/v1" || payload["cmd"] != "request.get" || payload["url"] != "https://www.fanbox.cc/" || len(payload) != 2 || r.Header.Get("Cookie") != "" {
			t.Error("solver target or credentials escaped the anonymous boundary")
		}
		solves.Add(1)
		fmt.Fprint(w, `{"status":"ok","solution":{"userAgent":"solved-agent","cookies":[{"name":"cf_clearance","value":"clearance-fixture"},{"name":"FANBOXSESSID","value":"discard-fixture"}]}}`)
	}))
	defer solver.Close()
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Cookie") == "FANBOXSESSID=session-fixture" {
			w.Header().Set("Cf-Mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Header.Get("Cookie") != "FANBOXSESSID=session-fixture; cf_clearance=clearance-fixture" || r.Header.Get("User-Agent") != "solved-agent" {
			t.Error("incorrect replay credentials or agent")
		}
		fmt.Fprint(w, `<meta name="metadata" content='{"context":{"user":{"userId":21,"name":"fixture"}}}'>`)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient, FlareSolverrURL: solver.URL})
	require.NoError(t, err)
	require.NoError(t, client.ValidateSession(t.Context()))
	require.NoError(t, client.ValidateSession(t.Context()))
	require.EqualValues(t, 1, solves.Load())
	require.EqualValues(t, 3, calls.Load())
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

func TestSolverCancellation(t *testing.T) {
	started := make(chan struct{})
	solver := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		close(started)
		<-r.Context().Done()
	}))
	defer solver.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient, FlareSolverrURL: solver.URL})
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

func TestUnavailableSolver(t *testing.T) {
	solver := httptest.NewServer(http.NotFoundHandler())
	solver.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient, FlareSolverrURL: solver.URL})
	require.NoError(t, err)
	err = client.ValidateSession(t.Context())
	require.Equal(t, sdk.UpstreamUnavailable, sdk.ReasonOf(err))
	require.NotContains(t, fmt.Sprintf("%#v", err), "session-fixture")
	require.NoError(t, client.Close(t.Context()))
}

func TestDelayedChallengeReusesFreshClearance(t *testing.T) {
	var solves atomic.Int32
	solver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		solves.Add(1)
		fmt.Fprint(w, `{"status":"ok","solution":{"userAgent":"fixture","cookies":[{"name":"cf_clearance","value":"clearance-fixture"}]}}`)
	}))
	defer solver.Close()
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Cookie"), "cf_clearance=") {
			fmt.Fprint(w, `{"body":{"user":{"name":"fixture"}}}`)
			return
		}
		if r.URL.Query().Get("creatorId") == "slow" {
			close(started)
			<-release
		}
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	httpClient := server.Client()
	routeToServer(httpClient, server)
	client, err := fanbox.New("session-fixture", fanbox.Options{HTTPClient: httpClient, FlareSolverrURL: solver.URL})
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() { _, err := client.Creator("slow").Get(t.Context()); result <- err }()
	<-started
	_, err = client.Creator("fast").Get(t.Context())
	close(release)
	require.NoError(t, err)
	require.NoError(t, <-result)
	require.EqualValues(t, 1, solves.Load())
	require.NoError(t, client.Close(t.Context()))
}
