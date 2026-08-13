package pixiv_test

import (
	"context"
	"sync"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPixivMCPDiagnosticsUseStableLocalRequestIDs(t *testing.T) {
	var (
		mu     sync.Mutex
		events []diagnostics.Event
	)
	sink := diagnostics.SinkFunc(func(event diagnostics.Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	})
	rootCtx, cancel := context.WithCancel(diagnostics.WithScope(context.Background(), sink, diagnostics.ModulePixivCLI, 0))
	defer cancel()

	server := mcp.NewServer(&mcp.Implementation{Name: "debug-test", Version: "1"}, nil)
	runtime.AddTool(runtime.NewApp(nil, nil, runtime.SDKPorts{}, runtime.Account{}), server, &mcp.Tool{Name: "diagnostic_test"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
		return &mcp.CallToolResult{}, struct{}{}, nil
	})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(rootCtx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "debug-client", Version: "1"}, nil)
	session, err := client.Connect(rootCtx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	callTool(t, session, "diagnostic_test", map[string]any{})
	callTool(t, session, "diagnostic_test", map[string]any{})

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 4 {
		t.Fatalf("events=%+v want two start/complete pairs", events)
	}
	for index, event := range events {
		wantID := uint64(index/2 + 1)
		if event.Module != diagnostics.ModulePixivMCPServer || event.RequestID != wantID {
			t.Fatalf("event[%d]=%+v", index, event)
		}
	}
}
