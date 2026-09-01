package ascii2d

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type closeTrackingTransport struct {
	closeCalls atomic.Int32
}

func (t *closeTrackingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected ascii2d request")
}

func (t *closeTrackingTransport) CloseIdleConnections() {
	t.closeCalls.Add(1)
}

func TestClientCloseDestroysSolverSessionAndClosesIdleConnectionsOnce(t *testing.T) {
	var destroyCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload solverRequest
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		switch payload.Command {
		case "sessions.create":
			writeSolverJSON(writer, `{"status":"ok"}`)
		case "sessions.destroy":
			destroyCalls.Add(1)
			writeSolverJSON(writer, `{"status":"ok"}`)
		default:
			t.Errorf("unexpected solver command %q", payload.Command)
			writeSolverJSONStatus(writer, http.StatusBadRequest, `{"status":"error"}`)
		}
	}))
	defer server.Close()

	transport := &closeTrackingTransport{}
	client, err := New(Options{
		HTTPClient: &http.Client{Transport: transport},
		FlareSolverr: &FlareSolverrOptions{
			URL: server.URL,
		},
		UserAgent: "fixture-user-agent",
	})
	require.NoError(t, err)
	require.NoError(t, client.solver.create(context.Background()))

	require.NoError(t, client.Close())
	require.NoError(t, client.Close())
	require.Equal(t, int32(1), destroyCalls.Load())
	require.Equal(t, int32(1), transport.closeCalls.Load())
}
