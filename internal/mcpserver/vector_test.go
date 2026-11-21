package mcpserver_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestVectorSearchInitialScanError(t *testing.T) {
	server := mcpserver.New(pixiv.SDKPorts{VectorSearch: func(context.Context, vectorindex.SearchInput, string, func(context.Context) (pixiv.Account, error)) (vectorindex.SearchResponse, error) {
		return vectorindex.SearchResponse{}, &vectorindex.SearchError{Code: "initial_sync_required", Cause: errors.New("private-canary")}
	}}, pixiv.Account{}, tools.SDKPorts{}, nil)
	c, s := mcp.NewInMemoryTransports()
	ss, err := server.Connect(t.Context(), s, nil)
	require.NoError(t, err)
	defer ss.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "vector-schema", Version: "1"}, nil).Connect(t.Context(), c, nil)
	require.NoError(t, err)
	defer session.Close()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "pixiv_vector_search", Arguments: map[string]any{"query": "a girl"}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Equal(t, "initial_sync_required", result.StructuredContent.(map[string]any)["error"])
	message := result.Content[0].(*mcp.TextContent).Text
	require.Contains(t, message, "pixiv vector sync")
	require.NotContains(t, message, "private-canary")
}
