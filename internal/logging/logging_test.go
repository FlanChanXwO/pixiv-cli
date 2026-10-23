package logging_test

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
)

func TestDisabledScopeDoesNotEmitOrCreateOutput(t *testing.T) {
	var output bytes.Buffer
	ctx := logging.WithScope(context.Background(), logging.Nop(), logging.ModulePixivCLI, 0)
	logging.Emit(ctx, logging.Event{Kind: logging.EventStarted, Operation: "pixiv config path"})
	if output.Len() != 0 {
		t.Fatalf("disabled diagnostics wrote %q", output.String())
	}
}

func TestScopeAddsModuleAndRequestID(t *testing.T) {
	var (
		mu     sync.Mutex
		events []logging.Event
	)
	sink := logging.SinkFunc(func(event logging.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	ctx := logging.WithScope(context.Background(), sink, logging.ModuleFanboxMCPServer, 7)
	logging.Emit(ctx, logging.Event{Kind: logging.EventNetworkRequest, Operation: "retrieve", Resource: "post 12221352"})

	if len(events) != 1 {
		t.Fatalf("event count=%d want 1", len(events))
	}
	if events[0].Module != logging.ModuleFanboxMCPServer || events[0].RequestID != 7 {
		t.Fatalf("event scope=%+v", events[0])
	}

	child := logging.WithChildScope(ctx, logging.ModuleFanboxSolver, 8)
	logging.Emit(child, logging.Event{Kind: logging.EventSolverStarted, Operation: "challenge recovery"})
	if len(events) != 2 || events[1].Module != logging.ModuleFanboxSolver || events[1].RequestID != 8 {
		t.Fatalf("child event scope=%+v", events)
	}
}
