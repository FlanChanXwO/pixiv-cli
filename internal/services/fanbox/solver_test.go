package fanbox

import (
	"context"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
)

func TestChallengeRecoveryUsesAnonymousSolverAndNativeReplay(t *testing.T) {
	var nativeCalls atomic.Int32
	var solverCalls atomic.Int32
	native := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		call := nativeCalls.Add(1)
		if request.URL.Host != "api.fanbox.cc" || request.URL.Path != "/post.info" {
			t.Fatalf("native request = %s", request.URL)
		}
		if call == 1 {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     http.Header{"Cf-Mitigated": {"challenge"}},
				Body:       io.NopCloser(strings.NewReader("challenge body")),
			}, nil
		}
		if got := request.Header.Get("User-Agent"); got != "solver-agent" {
			t.Errorf("replay User-Agent = %q", got)
		}
		cookie := request.Header.Get("Cookie")
		if !strings.Contains(cookie, "FANBOXSESSID=dummy-session") || !strings.Contains(cookie, "cf_clearance=clearance-value") {
			t.Errorf("replay Cookie = %q", cookie)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"body":{"post":{"id":"post-1","title":"replayed","publishedDatetime":"2024-01-01T00:00:00Z"}}}`)),
		}, nil
	})
	solver := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		solverCalls.Add(1)
		if request.URL.Path != "/v1" {
			t.Errorf("solver path = %q", request.URL.Path)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "dummy-session") || strings.Contains(string(body), "post.info") {
			t.Fatalf("solver received sensitive/business data: %q", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"status":"ok","solution":{"userAgent":"solver-agent","cookies":[{"name":"cf_clearance","value":"clearance-value"},{"name":"irrelevant","value":"discarded"}]}}`)),
		}, nil
	})
	session, err := NewSessionWithOptions("FANBOXSESSID=dummy-session", SessionOptions{
		HTTPClient:   &http.Client{Transport: native},
		FlareSolverr: &FlareSolverrOptions{URL: "http://solver.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.solverHTTPClient = &http.Client{Transport: solver}

	post, err := session.Post(context.Background(), "post-1")
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if post.ID != "post-1" || post.Title != "replayed" {
		t.Fatalf("post = %+v", post)
	}
	if got := solverCalls.Load(); got != 1 {
		t.Fatalf("solver calls = %d, want 1", got)
	}

	if _, err := session.Post(context.Background(), "post-1"); err != nil {
		t.Fatalf("cached replay Post() error = %v", err)
	}
	if got := solverCalls.Load(); got != 1 {
		t.Fatalf("cached solver calls = %d, want 1", got)
	}
}

func TestOrdinaryForbiddenDoesNotCallSolver(t *testing.T) {
	var solverCalls atomic.Int32
	session, err := NewSessionWithOptions("FANBOXSESSID=dummy-session", SessionOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"challenge"}`))}, nil
		})},
		FlareSolverr: &FlareSolverrOptions{URL: "http://solver.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.solverHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		solverCalls.Add(1)
		return nil, nil
	})}
	if _, err := session.Post(context.Background(), "post-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Post() error = %v, want forbidden", err)
	}
	if got := solverCalls.Load(); got != 0 {
		t.Fatalf("solver calls = %d, want 0", got)
	}
}

func TestMalformedSolverResponseIsNotCached(t *testing.T) {
	var solverCalls atomic.Int32
	native := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Cf-Mitigated": {"challenge"}}, Body: io.NopCloser(strings.NewReader("challenge"))}, nil
	})
	solver := roundTripFunc(func(*http.Request) (*http.Response, error) {
		solverCalls.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"status":"ok","solution":{"userAgent":"solver-agent","cookies":[{"name":"cf_clearance","value":"one"},{"name":"cf_clearance","value":"two"}]}}`))}, nil
	})
	session, err := NewSessionWithOptions("FANBOXSESSID=dummy-session", SessionOptions{
		HTTPClient:   &http.Client{Transport: native},
		FlareSolverr: &FlareSolverrOptions{URL: "http://solver.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.solverHTTPClient = &http.Client{Transport: solver}
	for range 2 {
		if _, err := session.Post(context.Background(), "post-1"); !errors.Is(err, ErrMalformedSolverResponse) {
			t.Fatalf("Post() error = %v, want malformed solver response", err)
		}
	}
	if got := solverCalls.Load(); got != 2 {
		t.Fatalf("solver calls = %d, want 2", got)
	}
}

func TestDiagnosticsObserveChallengeRecoveryWithoutSecrets(t *testing.T) {
	var (
		nativeCalls atomic.Int32
		mu          sync.Mutex
		events      []diagnostics.Event
	)
	native := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if nativeCalls.Add(1) == 1 {
			return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Cf-Mitigated": {"challenge"}}, Body: io.NopCloser(strings.NewReader("challenge"))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"body":{"post":{"id":"post-1","title":"replayed","publishedDatetime":"2024-01-01T00:00:00Z"}}}`))}, nil
	})
	solver := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"status":"ok","solution":{"userAgent":"solver-agent","cookies":[{"name":"cf_clearance","value":"clearance-secret"}]}}`))}, nil
	})
	sink := diagnostics.SinkFunc(func(event diagnostics.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	ctx := diagnostics.WithScope(context.Background(), sink, diagnostics.ModuleFanboxCLI, 4)
	session, err := NewSessionWithOptions("FANBOXSESSID=session-secret", SessionOptions{
		HTTPClient:   &http.Client{Transport: native},
		FlareSolverr: &FlareSolverrOptions{URL: "http://solver.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.solverHTTPClient = &http.Client{Transport: solver}
	if _, err := session.Post(ctx, "post-1"); err != nil {
		t.Fatalf("Post() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) < 5 {
		t.Fatalf("events=%+v want network/challenge/solver/replay lifecycle", events)
	}
	for _, event := range events {
		if event.RequestID != 4 {
			t.Fatalf("event request id=%d want 4", event.RequestID)
		}
		if strings.Contains(event.Resource, "?") || strings.Contains(event.Resource, "session-secret") || strings.Contains(event.Resource, "clearance-secret") {
			t.Fatalf("event leaked sensitive resource: %+v", event)
		}
	}
}

func TestConcurrentChallengeWaiterCancellationKeepsSharedSolve(t *testing.T) {
	var nativeCalls atomic.Int32
	native := roundTripFunc(func(*http.Request) (*http.Response, error) {
		if nativeCalls.Add(1) <= 2 {
			return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Cf-Mitigated": {"challenge"}}, Body: io.NopCloser(strings.NewReader("challenge"))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"body":{"post":{"id":"post-1","title":"replayed","publishedDatetime":"2024-01-01T00:00:00Z"}}}`))}, nil
	})
	var solverCalls atomic.Int32
	solverStarted := make(chan struct{})
	var solverStartedOnce sync.Once
	releaseSolver := make(chan struct{})
	solver := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		solverCalls.Add(1)
		solverStartedOnce.Do(func() { close(solverStarted) })
		select {
		case <-releaseSolver:
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"status":"ok","solution":{"userAgent":"solver-agent","cookies":[{"name":"cf_clearance","value":"clearance"}]}}`))}, nil
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	})
	session := newSolverTestSession(t, native, solver)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	type result struct{ err error }
	firstResult := make(chan result, 1)
	go func() {
		_, err := session.Post(firstCtx, "post-1")
		firstResult <- result{err: err}
	}()
	select {
	case <-solverStarted:
	case <-testTimeout(t):
		t.Fatal("solver did not start")
	}

	secondResult := make(chan result, 1)
	go func() {
		_, err := session.Post(context.Background(), "post-1")
		secondResult <- result{err: err}
	}()
	waitForSolverWaiters(t, session, 2)
	cancelFirst()
	close(releaseSolver)

	select {
	case got := <-firstResult:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("first waiter error=%v, want context cancellation", got.err)
		}
	case <-testTimeout(t):
		t.Fatal("first waiter did not finish")
	}
	select {
	case got := <-secondResult:
		if got.err != nil {
			t.Fatalf("follower error=%v", got.err)
		}
	case <-testTimeout(t):
		t.Fatal("follower did not finish")
	}
	if got := solverCalls.Load(); got != 1 {
		t.Fatalf("solver calls=%d want one shared solve", got)
	}
}

func TestAllCanceledSolveIsNotCached(t *testing.T) {
	var solverCalls atomic.Int32
	solverStarted := make(chan struct{})
	solver := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		solverCalls.Add(1)
		close(solverStarted)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	native := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Cf-Mitigated": {"challenge"}}, Body: io.NopCloser(strings.NewReader("challenge"))}, nil
	})
	session := newSolverTestSession(t, native, solver)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := session.Post(ctx, "post-1")
		result <- err
	}()
	select {
	case <-solverStarted:
	case <-testTimeout(t):
		t.Fatal("solver did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v want context cancellation", err)
		}
	case <-testTimeout(t):
		t.Fatal("canceled waiter did not finish")
	}
	if session.solverState != nil {
		t.Fatal("all-canceled solve was cached")
	}
	if got := solverCalls.Load(); got != 1 {
		t.Fatalf("solver calls=%d want one", got)
	}
}

func newSolverTestSession(t *testing.T, native, solver http.RoundTripper) *Session {
	t.Helper()
	session, err := NewSessionWithOptions("FANBOXSESSID=dummy-session", SessionOptions{
		HTTPClient:   &http.Client{Transport: native},
		FlareSolverr: &FlareSolverrOptions{URL: "http://solver.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	session.solverHTTPClient = &http.Client{Transport: solver}
	return session
}

func waitForSolverWaiters(t *testing.T, session *Session, want int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		session.solverMu.Lock()
		ready := session.solverActive != nil && session.solverActive.waiters == want
		session.solverMu.Unlock()
		if ready {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("solver waiters did not reach %d", want)
		default:
			runtime.Gosched()
		}
	}
}

func testTimeout(t *testing.T) <-chan time.Time {
	t.Helper()
	return time.After(2 * time.Second)
}
