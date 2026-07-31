package mcpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLegacySearchFailurePreservesStructuredErrorResult(t *testing.T) {
	typedErr := &sdk.Error{
		Code:           sdk.CodeUpstreamError,
		Operation:      sdk.OperationSearchIllust,
		Backend:        sdk.BackendAppAPI,
		UpstreamStatus: http.StatusBadGateway,
	}
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return &fakeSDKClient{searchIllust: func(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
			return nil, typedErr
		}}, nil
	}}
	server := NewWithSDK(&fakeAPI{}, &fakeDownloads{}, service, application.SDKClientRequest{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result := callTool(t, session, "search_illust", map[string]any{"word": "ordinary-query"})
	if !result.IsError || !resultHasText(result, "Error: "+typedErr.Error()) {
		t.Fatalf("structured search failure changed: %+v", result)
	}
	var out illustQueryOut
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 {
		t.Fatalf("structured output=%+v, want empty records", out)
	}
}

func TestLegacyDownloadValidationPreservesStructuredErrorResult(t *testing.T) {
	server := New(&fakeAPI{}, &fakeDownloads{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result := callTool(t, session, "download", map[string]any{})
	assertEmptyDownloadResult(t, result, deliveryLocalPath, "Error: provide src (one source) or srcs (a source list)")
}
