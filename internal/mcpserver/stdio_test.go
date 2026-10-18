package mcpserver_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestRunStdioCancellationWaitsForToolCleanup(t *testing.T) {
	for _, eof := range []bool{false, true} {
		name := "signal"
		if eof {
			name = "EOF"
		}
		t.Run(name, func(t *testing.T) { testStdioCleanup(t, eof) })
	}
}

func testStdioCleanup(t *testing.T, eof bool) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := mcp.NewServer(&mcp.Implementation{Name: "stdio-lifecycle", Version: "1"}, nil)
	started, canceled, cleanup := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server.AddTool(&mcp.Tool{Name: "wait", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-cleanup
		return nil, ctx.Err()
	})
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	defer serverRead.Close()
	defer clientRead.Close()
	defer serverWrite.Close()
	defer clientWrite.Close()
	done := make(chan error, 1)
	go func() { done <- mcpserver.RunStdio(ctx, server, serverRead, serverWrite) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "lifecycle-client", Version: "1"}, nil).Connect(t.Context(), &mcp.IOTransport{Reader: clientRead, Writer: clientWrite}, nil)
	require.NoError(t, err)
	defer session.Close()
	called := make(chan struct{})
	go func() {
		defer close(called)
		_, _ = session.CallTool(t.Context(), &mcp.CallToolParams{Name: "wait", Arguments: map[string]any{}})
	}()
	<-started
	if eof {
		require.NoError(t, clientWrite.Close())
	} else {
		cancel()
	}
	<-canceled
	select {
	case err := <-done:
		t.Fatalf("returned before handler cleanup: %v", err)
	default:
	}
	close(cleanup)
	if eof {
		require.NoError(t, <-done)
	} else {
		require.True(t, errors.Is(<-done, context.Canceled))
	}
	// Process exit closes stdout after the runner has joined every handler.
	require.NoError(t, serverWrite.Close())
	require.NoError(t, session.Close())
	<-called
}
