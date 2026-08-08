package diagnostics

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDisabledScopeDoesNotEmitOrCreateOutput(t *testing.T) {
	var output bytes.Buffer
	ctx := WithScope(context.Background(), Nop(), ModulePixivCLI, 0)
	Emit(ctx, Event{Kind: EventStarted, Operation: "pixiv config path"})
	if output.Len() != 0 {
		t.Fatalf("disabled diagnostics wrote %q", output.String())
	}
}

func TestScopeAddsModuleAndRequestID(t *testing.T) {
	var (
		mu     sync.Mutex
		events []Event
	)
	sink := SinkFunc(func(event Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	ctx := WithScope(context.Background(), sink, ModuleFanboxMCPServer, 7)
	Emit(ctx, Event{Kind: EventNetworkRequest, Operation: "retrieve", Resource: "post 12221352"})

	if len(events) != 1 {
		t.Fatalf("event count=%d want 1", len(events))
	}
	if events[0].Module != ModuleFanboxMCPServer || events[0].RequestID != 7 {
		t.Fatalf("event scope=%+v", events[0])
	}

	child := WithChildScope(ctx, ModuleFanboxSolver, 8)
	Emit(child, Event{Kind: EventSolverStarted, Operation: "challenge recovery"})
	if len(events) != 2 || events[1].Module != ModuleFanboxSolver || events[1].RequestID != 8 {
		t.Fatalf("child event scope=%+v", events)
	}
}

func TestPresenterUsesStableNarrativeAndClock(t *testing.T) {
	var output bytes.Buffer
	clock := func() time.Time {
		return time.Date(2026, time.August, 8, 12, 21, 18, 0, time.FixedZone("CST", 8*60*60))
	}
	presenter := NewPresenterWithClock(&output, clock)

	presenter.Emit(Event{Module: ModuleFanboxMCPServer, Kind: EventStarted, Operation: "tool fanbox_get_post", RequestID: 7})
	presenter.Emit(Event{Module: ModuleFanboxNetwork, Kind: EventNetworkRequest, Operation: "retrieving", Resource: "post 12221352", Route: "native transport", RequestID: 7})
	presenter.Emit(Event{Module: ModuleFanboxNetwork, Kind: EventChallenge, Operation: "request", Status: 403, RequestID: 7})
	presenter.Emit(Event{Module: ModuleFanboxSolver, Kind: EventSolverStarted, Operation: "challenge recovery", RequestID: 7})
	presenter.Emit(Event{Module: ModuleFanboxSolver, Kind: EventSolverCompleted, Operation: "clearance", RequestID: 7})
	presenter.Emit(Event{Module: ModuleFanboxNetwork, Kind: EventReplay, Operation: "request", Route: "native transport", RequestID: 7})
	presenter.Emit(Event{Module: ModuleFanboxMCPServer, Kind: EventCompleted, Operation: "tool fanbox_get_post", RequestID: 7, Duration: 16*time.Second + 200*time.Millisecond})

	want := strings.Join([]string{
		"[FANBOX MCP server] 12:21:18 Started request 7 for tool fanbox_get_post.",
		"[FANBOX network] 12:21:18 Request 7 is retrieving post 12221352 through the native transport.",
		"[FANBOX network] 12:21:18 Cloudflare challenged request 7 with HTTP 403.",
		"[FANBOX FlareSolverr] 12:21:18 Request 7 requires fresh Cloudflare clearance.",
		"[FANBOX FlareSolverr] 12:21:18 Clearance was acquired; request 7 will be replayed natively.",
		"[FANBOX network] 12:21:18 Request 7 is replaying through the native transport.",
		"[FANBOX MCP server] 12:21:18 Request 7 completed successfully in 16.2 seconds.",
	}, "\n") + "\n"
	if output.String() != want {
		t.Fatalf("narrative output=\n%s\nwant=\n%s", output.String(), want)
	}
}

func TestPresenterRemovesSensitiveURLParts(t *testing.T) {
	var output bytes.Buffer
	presenter := NewPresenterWithClock(&output, func() time.Time { return time.Unix(0, 0) })
	presenter.Emit(Event{
		Module:    ModuleFanboxNetwork,
		Kind:      EventNetworkRequest,
		Operation: "retrieving",
		Resource:  "https://user:secret@example.test/post/1?signature=secret#fragment",
		Route:     "native transport",
		Proxy:     "https://proxy-user:proxy-secret@proxy.test:7890?token=secret",
		UserAgent: "Mozilla/5.0\nAuthorization: secret",
	})

	got := output.String()
	for _, secret := range []string{"secret", "proxy-user", "Authorization:"} {
		if strings.Contains(got, secret) {
			t.Fatalf("diagnostic output leaked %q: %s", secret, got)
		}
	}
	if strings.Contains(got, "?signature=") || strings.Contains(got, "#fragment") {
		t.Fatalf("diagnostic output retained signed URL parts: %s", got)
	}
}

func TestPresenterRetainsFirstWriterError(t *testing.T) {
	writer := &errorWriter{err: errors.New("diagnostic sink closed")}
	presenter := NewPresenter(writer)
	presenter.Emit(Event{Module: ModulePixivCLI, Kind: EventStarted, Operation: "pixiv version"})
	presenter.Emit(Event{Module: ModulePixivCLI, Kind: EventCompleted, Operation: "pixiv version"})
	if !errors.Is(presenter.Err(), writer.err) {
		t.Fatalf("presenter error=%v want %v", presenter.Err(), writer.err)
	}
}

type errorWriter struct{ err error }

func (w *errorWriter) Write([]byte) (int, error) { return 0, w.err }
