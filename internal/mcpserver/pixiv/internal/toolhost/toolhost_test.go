package toolhost_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestToolFailureLogKeepsCredentialsPrivate(t *testing.T) {
	for _, format := range []string{logging.FormatText, logging.FormatJSON} {
		t.Run(format, func(t *testing.T) {
			var log bytes.Buffer
			logger, err := logging.New(&log, "error", format)
			require.NoError(t, err)
			app := toolhost.NewApp(toolhost.SDKPorts{Logger: logger}, toolhost.Account{})
			server := mcp.NewServer(&mcp.Implementation{Name: "privacy", Version: "1"}, nil)
			toolhost.AddTool(app, server, &mcp.Tool{Name: "fail"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
				return nil, struct{}{}, errors.Join(errors.New("Cookie=cookie-canary"), errors.New("https://media.test/a?signature=signature-canary"))
			})
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(t.Context(), serverTransport, nil)
			require.NoError(t, err)
			defer serverSession.Close()
			session, err := mcp.NewClient(&mcp.Implementation{Name: "privacy", Version: "1"}, nil).Connect(t.Context(), clientTransport, nil)
			require.NoError(t, err)
			defer session.Close()
			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "fail", Arguments: map[string]any{}})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Contains(t, log.String(), "Tool failed")
			for _, secret := range []string{"cookie-canary", "signature-canary", "https://media.test/a"} {
				require.NotContains(t, log.String(), secret)
			}
		})
	}
}
