package ascii2d

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestSolverClientUsesProxyOnlyWhenCreatingSessionAndParsesSolution(t *testing.T) {
	const (
		sessionName = "ascii2d-fixture-session"
		proxyURL    = "socks5://solver-proxy.invalid:1080"
		targetURL   = "https://ascii2d.net/search/color/protected"
	)

	type capturedCall struct {
		command string
		body    map[string]any
	}
	var calls []capturedCall
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1" {
			t.Errorf("solver endpoint path = %q, want /v1", request.URL.Path)
		}
		if request.Method != http.MethodPost {
			t.Errorf("solver method = %q, want POST", request.Method)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("solver content type = %q, want application/json", got)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Errorf("solver accept = %q, want application/json", got)
		}

		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read solver request: %v", err)
			return
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Errorf("decode solver request: %v", err)
			return
		}
		command, _ := decoded["cmd"].(string)
		calls = append(calls, capturedCall{command: command, body: decoded})

		switch command {
		case "sessions.create":
			proxy, ok := decoded["proxy"].(map[string]any)
			if !ok || proxy["url"] != proxyURL {
				t.Errorf("session create proxy = %#v, want object containing %q", decoded["proxy"], proxyURL)
			}
			if decoded["url"] != nil {
				t.Errorf("session create unexpectedly contains protected URL: %#v", decoded["url"])
			}
			writeSolverJSON(writer, `{"status":"ok","message":"Session created."}`)
		case "request.get":
			if decoded["proxy"] != nil {
				t.Errorf("request.get unexpectedly contains solver proxy: %#v", decoded["proxy"])
			}
			if decoded["session"] != sessionName {
				t.Errorf("request.get session = %#v, want %q", decoded["session"], sessionName)
			}
			if decoded["url"] != targetURL {
				t.Errorf("request.get URL = %#v, want %q", decoded["url"], targetURL)
			}
			if timeout, ok := decoded["maxTimeout"].(float64); !ok || timeout != float64(flareSolverrMaxTimeout) {
				t.Errorf("request.get maxTimeout = %#v, want %d", decoded["maxTimeout"], flareSolverrMaxTimeout)
			}
			writeSolverJSON(writer, `{"status":"ok","solution":{"userAgent":"Mozilla/5.0 (X11; Linux x86_64) Chrome/146.0.0.0","cookies":[{"name":"other","value":"discard-me"},{"name":"cf_clearance","value":"clearance-fixture","expires":"2030-01-02T03:04:05Z"}]}}`)
		case "sessions.destroy":
			if decoded["proxy"] != nil || decoded["url"] != nil {
				t.Errorf("session destroy contains request-only fields: %#v", decoded)
			}
			if decoded["session"] != sessionName {
				t.Errorf("session destroy session = %#v, want %q", decoded["session"], sessionName)
			}
			writeSolverJSON(writer, `{"status":"ok","message":"Session destroyed."}`)
		default:
			t.Errorf("unexpected solver command %q", command)
			writeSolverJSONStatus(writer, http.StatusBadRequest, `{"status":"error"}`)
		}
	}))
	defer server.Close()

	client, err := newSolverClient(solverClientOptions{
		FlareSolverr: FlareSolverrOptions{URL: server.URL, ProxyURL: proxyURL},
		SessionName:  sessionName,
	})
	if err != nil {
		t.Fatalf("newSolverClient() error = %v", err)
	}
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatalf("solver control transport = %#v, want direct http.Transport", client.httpClient.Transport)
	}

	ctx := context.Background()
	if err := client.create(ctx); err != nil {
		t.Fatalf("create() error = %v", err)
	}
	solution, err := client.get(ctx, targetURL)
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if solution.userAgent == "" || solution.clearance != "clearance-fixture" {
		t.Fatalf("solution = %+v, want solver UA and clearance", solution)
	}
	if !solution.hasExpiry || !solution.expiresAt.Equal(time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("solution expiry = %v (has=%v), want fixture expiry", solution.expiresAt, solution.hasExpiry)
	}
	if err := client.destroy(ctx); err != nil {
		t.Fatalf("destroy() error = %v", err)
	}

	if len(calls) != 3 {
		t.Fatalf("solver call count = %d, want 3", len(calls))
	}
	if got := []string{calls[0].command, calls[1].command, calls[2].command}; strings.Join(got, ",") != "sessions.create,request.get,sessions.destroy" {
		t.Fatalf("solver commands = %v", got)
	}
}

func TestSolverClientMapsProtocolFailuresWithoutLeakingResponseBody(t *testing.T) {
	const secret = "clearance-secret-body-canary"
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       error
	}{
		{name: "transport failure", want: ErrSolverUnavailable},
		{name: "http failure", statusCode: http.StatusBadGateway, body: `{"status":"error","message":"` + secret + `"}`, want: ErrSolverFailed},
		{name: "solver status failure", statusCode: http.StatusOK, body: `{"status":"error","message":"` + secret + `"}`, want: ErrSolverFailed},
		{name: "unknown solver status", statusCode: http.StatusOK, body: `{"status":"pending"}`, want: ErrMalformedSolverResponse},
		{name: "malformed json", statusCode: http.StatusOK, body: `{"status":"ok","solution":`, want: ErrMalformedSolverResponse},
		{name: "trailing json", statusCode: http.StatusOK, body: `{"status":"ok"}{"status":"ok"}`, want: ErrMalformedSolverResponse},
		{name: "missing solution", statusCode: http.StatusOK, body: `{"status":"ok"}`, want: ErrMalformedSolverResponse},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var client *solverClient
			if test.name == "transport failure" {
				client, _ = newSolverClient(solverClientOptions{
					FlareSolverr: FlareSolverrOptions{URL: "http://solver.invalid"},
					HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
						return nil, errors.New("transport details containing " + secret)
					})},
					SessionName: "fixture-session",
				})
			} else {
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					writeSolverJSONStatus(writer, test.statusCode, test.body)
				}))
				defer server.Close()
				client, _ = newSolverClient(solverClientOptions{
					FlareSolverr: FlareSolverrOptions{URL: server.URL},
					SessionName:  "fixture-session",
				})
			}

			_, err := client.get(context.Background(), "https://ascii2d.net/")
			if !errors.Is(err, test.want) {
				t.Fatalf("get() error = %v, want errors.Is(..., %v)", err, test.want)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("get() error leaks solver response/transport secret: %q", err)
			}
		})
	}
}

func TestValidateSolverSolutionRejectsMissingOrConflictingClearance(t *testing.T) {
	tests := []struct {
		name     string
		solution *solverResponseSolution
	}{
		{name: "nil solution"},
		{name: "missing user agent", solution: &solverResponseSolution{Cookies: []solverCookie{{Name: "cf_clearance", Value: "clearance"}}}},
		{name: "missing clearance", solution: &solverResponseSolution{UserAgent: "solver-agent"}},
		{name: "duplicate clearance", solution: &solverResponseSolution{UserAgent: "solver-agent", Cookies: []solverCookie{{Name: "cf_clearance", Value: "one"}, {Name: "cf_clearance", Value: "two"}}}},
		{name: "invalid clearance", solution: &solverResponseSolution{UserAgent: "solver-agent", Cookies: []solverCookie{{Name: "cf_clearance", Value: "bad;value"}}}},
		{name: "header injection", solution: &solverResponseSolution{UserAgent: "solver-agent\r\nX-Leak: yes", Cookies: []solverCookie{{Name: "cf_clearance", Value: "clearance"}}}},
		{name: "invalid expiry", solution: &solverResponseSolution{UserAgent: "solver-agent", Cookies: []solverCookie{{Name: "cf_clearance", Value: "clearance", Expires: json.RawMessage(`"not-a-date"`)}}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := validateSolverSolution(test.solution); !errors.Is(err, ErrMalformedSolverResponse) {
				t.Fatalf("validateSolverSolution() error = %v, want %v", err, ErrMalformedSolverResponse)
			}
		})
	}
}

func TestSolverClientCreateIsIdempotent(t *testing.T) {
	var createCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read solver request: %v", err)
		}
		var decoded solverRequest
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode solver request: %v", err)
		}
		if decoded.Command != "sessions.create" {
			t.Fatalf("solver command = %q, want sessions.create", decoded.Command)
		}
		createCalls++
		writeSolverJSON(writer, `{"status":"ok"}`)
	}))
	defer server.Close()

	client, err := newSolverClient(solverClientOptions{
		FlareSolverr: FlareSolverrOptions{URL: server.URL},
		SessionName:  "fixture-session",
	})
	if err != nil {
		t.Fatalf("newSolverClient() error = %v", err)
	}

	ctx := context.Background()
	if err := client.create(ctx); err != nil {
		t.Fatalf("first create() error = %v", err)
	}
	if err := client.create(ctx); err != nil {
		t.Fatalf("second create() error = %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("sessions.create calls = %d, want 1", createCalls)
	}
}

func writeSolverJSON(writer http.ResponseWriter, body string) {
	writeSolverJSONStatus(writer, http.StatusOK, body)
}

func writeSolverJSONStatus(writer http.ResponseWriter, status int, body string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, body)
}
