package ascii2d

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestSolverStateCacheReusesStateUntilExpiryOrInvalidation(t *testing.T) {
	now := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	cache := newSolverStateCache()
	cache.now = func() time.Time { return now }

	var calls atomic.Int32
	solve := func(context.Context) (solverState, error) {
		calls.Add(1)
		return solverState{
			userAgent: "solver-agent",
			clearance: "clearance-fixture",
			expiresAt: now.Add(time.Hour),
			hasExpiry: true,
		}, nil
	}

	first, err := cache.getOrSolve(context.Background(), solve)
	if err != nil {
		t.Fatalf("first getOrSolve() error = %v", err)
	}
	second, err := cache.getOrSolve(context.Background(), solve)
	if err != nil {
		t.Fatalf("cached getOrSolve() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("solver calls before expiry = %d, want 1", calls.Load())
	}
	if first != second {
		t.Fatalf("cached solver state = %+v, want %+v", second, first)
	}

	now = now.Add(time.Hour)
	if _, err := cache.getOrSolve(context.Background(), solve); err != nil {
		t.Fatalf("expired getOrSolve() error = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("solver calls after expiry = %d, want 2", calls.Load())
	}

	cache.invalidate()
	if _, err := cache.getOrSolve(context.Background(), solve); err != nil {
		t.Fatalf("invalidated getOrSolve() error = %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("solver calls after invalidation = %d, want 3", calls.Load())
	}

	noExpiryCache := newSolverStateCache()
	var noExpiryCalls atomic.Int32
	noExpirySolve := func(context.Context) (solverState, error) {
		noExpiryCalls.Add(1)
		return solverState{userAgent: "solver-agent", clearance: "clearance-fixture"}, nil
	}
	if _, err := noExpiryCache.getOrSolve(context.Background(), noExpirySolve); err != nil {
		t.Fatalf("no-expiry first getOrSolve() error = %v", err)
	}
	if _, err := noExpiryCache.getOrSolve(context.Background(), noExpirySolve); err != nil {
		t.Fatalf("no-expiry cached getOrSolve() error = %v", err)
	}
	if noExpiryCalls.Load() != 1 {
		t.Fatalf("no-expiry solver calls = %d, want 1", noExpiryCalls.Load())
	}
}

func TestSolverStateCacheSharesConcurrentSolve(t *testing.T) {
	cache := newSolverStateCache()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	solve := func(ctx context.Context) (solverState, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return solverState{userAgent: "solver-agent", clearance: "clearance-fixture"}, nil
		case <-ctx.Done():
			return solverState{}, ctx.Err()
		}
	}

	results := make(chan error, 2)
	go func() {
		_, err := cache.getOrSolve(context.Background(), solve)
		results <- err
	}()
	<-started
	go func() {
		_, err := cache.getOrSolve(context.Background(), solve)
		results <- err
	}()
	waitForSolverWaiters(t, cache, 2)
	close(release)

	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent getOrSolve() error = %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("solver calls = %d, want 1", calls.Load())
	}
}

func TestSolverStateCacheWaiterCancellationDoesNotCancelSharedSolve(t *testing.T) {
	cache := newSolverStateCache()
	started := make(chan struct{})
	release := make(chan struct{})
	underlyingCanceled := make(chan struct{})
	var calls atomic.Int32
	solve := func(ctx context.Context) (solverState, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return solverState{userAgent: "solver-agent", clearance: "clearance-fixture"}, nil
		case <-ctx.Done():
			close(underlyingCanceled)
			return solverState{}, ctx.Err()
		}
	}

	firstResult := make(chan error, 1)
	go func() {
		_, err := cache.getOrSolve(context.Background(), solve)
		firstResult <- err
	}()
	<-started

	waiterContext, cancelWaiter := context.WithCancel(context.Background())
	secondResult := make(chan error, 1)
	go func() {
		_, err := cache.getOrSolve(waiterContext, solve)
		secondResult <- err
	}()
	waitForSolverWaiters(t, cache, 2)
	cancelWaiter()

	if err := <-secondResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter error = %v, want context.Canceled", err)
	}
	waitForSolverWaiters(t, cache, 1)
	select {
	case <-underlyingCanceled:
		t.Fatal("cancelled waiter cancelled the shared solve")
	default:
	}

	close(release)
	if err := <-firstResult; err != nil {
		t.Fatalf("remaining waiter error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("solver calls = %d, want 1", calls.Load())
	}
}

func TestSolverStateCacheCloseInvalidatesStateAndCancelsInFlightCall(t *testing.T) {
	cache := newSolverStateCache()
	var calls atomic.Int32
	solve := func(context.Context) (solverState, error) {
		calls.Add(1)
		return solverState{userAgent: "solver-agent", clearance: "clearance-fixture"}, nil
	}
	if _, err := cache.getOrSolve(context.Background(), solve); err != nil {
		t.Fatalf("initial getOrSolve() error = %v", err)
	}
	cache.close()
	cache.close()
	if _, err := cache.getOrSolve(context.Background(), solve); !errors.Is(err, errSolverClosed) {
		t.Fatalf("closed cache error = %v, want %v", err, errSolverClosed)
	}
	if calls.Load() != 1 {
		t.Fatalf("closed cache solver calls = %d, want 1", calls.Load())
	}

	inFlightCache := newSolverStateCache()
	started := make(chan struct{})
	inFlightResult := make(chan error, 1)
	go func() {
		_, err := inFlightCache.getOrSolve(context.Background(), func(ctx context.Context) (solverState, error) {
			close(started)
			<-ctx.Done()
			return solverState{}, ctx.Err()
		})
		inFlightResult <- err
	}()
	<-started
	waitForSolverWaiters(t, inFlightCache, 1)
	inFlightCache.close()
	if err := <-inFlightResult; !errors.Is(err, errSolverClosed) {
		t.Fatalf("in-flight close error = %v, want %v", err, errSolverClosed)
	}
	if _, err := inFlightCache.getOrSolve(context.Background(), solve); !errors.Is(err, errSolverClosed) {
		t.Fatalf("post-close in-flight cache error = %v, want %v", err, errSolverClosed)
	}
}

func TestSolverStateCacheDoesNotCacheFailure(t *testing.T) {
	cache := newSolverStateCache()
	var calls atomic.Int32
	solve := func(context.Context) (solverState, error) {
		if calls.Add(1) == 1 {
			return solverState{}, ErrSolverFailed
		}
		return solverState{userAgent: "solver-agent", clearance: "clearance-fixture"}, nil
	}
	if _, err := cache.getOrSolve(context.Background(), solve); !errors.Is(err, ErrSolverFailed) {
		t.Fatalf("first failure = %v, want %v", err, ErrSolverFailed)
	}
	if _, err := cache.getOrSolve(context.Background(), solve); err != nil {
		t.Fatalf("retry after failure = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("solver calls after failure = %d, want 2", calls.Load())
	}
}

func waitForSolverWaiters(t *testing.T, cache *solverStateCache, want int) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		cache.mu.Lock()
		ready := cache.active != nil && cache.active.waiters >= want
		cache.mu.Unlock()
		if ready {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("solver waiters did not reach %d", want)
			return
		default:
			runtime.Gosched()
		}
	}
}

func TestNewSessionClientSharesSolverStateCache(t *testing.T) {
	base, err := New(Options{HTTPClient: &http.Client{}, Endpoint: "https://ascii2d.test"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	session, err := base.newSessionClient()
	if err != nil {
		t.Fatalf("newSessionClient() error = %v", err)
	}
	if base.solverCache == nil {
		t.Fatal("base client has no solver state cache")
	}
	if session.solverCache != base.solverCache {
		t.Fatal("session client does not share the MCP-lifetime solver state cache")
	}
}
