package pixiv_test

import (
	"context"
	"net/http"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLegacySearchFailurePreservesStructuredErrorResult(t *testing.T) {
	typedErr := &sdk.Error{
		Product:    "pixiv",
		Operation:  "SearchArtworks",
		Reason:     sdk.UpstreamError,
		HTTPStatus: http.StatusBadGateway,
	}
	client := &fakeSDKClient{searchIllust: func(context.Context, pixiv.SearchArtworksRequest) (sdk.Page[pixiv.Artwork], error) {
		return sdk.Page[pixiv.Artwork]{}, typedErr
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "search_illust", map[string]any{"word": "ordinary-query"})
	if !result.IsError || !resultHasText(result, "Error: "+typedErr.Error()) {
		t.Fatalf("structured search failure changed: %+v", result)
	}
	var out outputs.Records
	decodeStructured(t, result, &out)
	if len(out.Records) != 0 {
		t.Fatalf("structured output=%+v, want empty records", out)
	}
}

func TestLegacyDownloadValidationPreservesStructuredErrorResult(t *testing.T) {
	server := pixivmcpserver.New(&fakeAPI{}, &fakeDownloads{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	result := callTool(t, session, "download", map[string]any{})
	assertEmptyDownloadResult(t, result, "local_path", "Error: provide src (one source) or srcs (a source list)")
}
