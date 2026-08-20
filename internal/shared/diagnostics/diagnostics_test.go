package diagnostics_test

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
)

func TestDisabledScopeDoesNotEmitOrCreateOutput(t *testing.T) {
	var output bytes.Buffer
	ctx := diagnostics.WithScope(context.Background(), diagnostics.Nop(), diagnostics.ModulePixivCLI, 0)
	diagnostics.Emit(ctx, diagnostics.Event{Kind: diagnostics.EventStarted, Operation: "pixiv config path"})
	if output.Len() != 0 {
		t.Fatalf("disabled diagnostics wrote %q", output.String())
	}
}

func TestScopeAddsModuleAndRequestID(t *testing.T) {
	var (
		mu     sync.Mutex
		events []diagnostics.Event
	)
	sink := diagnostics.SinkFunc(func(event diagnostics.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	ctx := diagnostics.WithScope(context.Background(), sink, diagnostics.ModuleFanboxMCPServer, 7)
	diagnostics.Emit(ctx, diagnostics.Event{Kind: diagnostics.EventNetworkRequest, Operation: "retrieve", Resource: "post 12221352"})

	if len(events) != 1 {
		t.Fatalf("event count=%d want 1", len(events))
	}
	if events[0].Module != diagnostics.ModuleFanboxMCPServer || events[0].RequestID != 7 {
		t.Fatalf("event scope=%+v", events[0])
	}

	child := diagnostics.WithChildScope(ctx, diagnostics.ModuleFanboxSolver, 8)
	diagnostics.Emit(child, diagnostics.Event{Kind: diagnostics.EventSolverStarted, Operation: "challenge recovery"})
	if len(events) != 2 || events[1].Module != diagnostics.ModuleFanboxSolver || events[1].RequestID != 8 {
		t.Fatalf("child event scope=%+v", events)
	}
}
