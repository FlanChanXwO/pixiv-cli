package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestLegacySearchFailurePreservesWireResultAndLogsTypedError 通过公开 MCP
// CallTool 路径锁定 legacy wire compatibility，并验证失败只进入安全诊断事件。
func TestLegacySearchFailurePreservesWireResultAndLogsTypedError(t *testing.T) {
	const queryCanary = "query-secret-canary https://secret.example/token?access_token=token-secret"
	typedErr := &sdk.Error{
		Code:           sdk.CodeUpstreamError,
		Operation:      sdk.OperationSearchIllust,
		Backend:        sdk.BackendAppAPI,
		UpstreamStatus: http.StatusBadGateway,
	}
	client := &fakeSDKClient{searchIllust: func(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		return nil, typedErr
	}}
	var logs bytes.Buffer
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return client, nil
	}}
	server := NewWithSDK(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewJSONHandler(&logs, nil)), service, application.SDKClientRequest{})
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

	result := callTool(t, session, "search_illust", map[string]any{"word": queryCanary})
	if result.IsError {
		t.Fatalf("legacy search failure changed isError: %+v", result)
	}
	wantOut := textOut{Text: typedErr.Error()}
	var gotOut textOut
	decodeStructured(t, result, &gotOut)
	if gotOut != wantOut {
		t.Fatalf("structured output=%+v, want %+v", gotOut, wantOut)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content len=%d, want 1", len(result.Content))
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]=%T, want *mcp.TextContent", result.Content[0])
	}
	var contentOut textOut
	if err := json.Unmarshal([]byte(textContent.Text), &contentOut); err != nil {
		t.Fatalf("decode text content %q: %v", textContent.Text, err)
	}
	if contentOut != wantOut {
		t.Fatalf("text content=%+v, want %+v", contentOut, wantOut)
	}

	event := findOperationEvent(t, logs.String(), "search_illust")
	for key, want := range map[string]any{
		"level":      "ERROR",
		"result":     "error",
		"error_code": string(sdk.CodeUpstreamError),
		"backend":    string(sdk.BackendAppAPI),
		"status":     float64(http.StatusBadGateway),
	} {
		if event[key] != want {
			t.Fatalf("search event %s=%v, want %v; event=%v", key, event[key], want, event)
		}
	}
	for _, secret := range []string{"query-secret-canary", "secret.example", "access_token", "token-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("MCP log leaked %q: %s", secret, logs.String())
		}
	}
}

func TestLegacySearchFailureRejectsHostileTypedLogMetadataWithoutChangingWire(t *testing.T) {
	const (
		codeCanary    = "code-token-secret"
		backendCanary = "https://secret.example/?token=backend-secret"
	)
	typedErr := &sdk.Error{
		Code:           sdk.ErrorCode(codeCanary),
		Operation:      sdk.OperationSearchIllust,
		Backend:        sdk.Backend(backendCanary),
		UpstreamStatus: http.StatusBadGateway,
	}
	client := &fakeSDKClient{searchIllust: func(context.Context, sdk.SearchIllustRequest) (*sdk.IllustListResult, error) {
		return nil, typedErr
	}}
	var logs bytes.Buffer
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return client, nil
	}}
	server := NewWithSDK(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewJSONHandler(&logs, nil)), service, application.SDKClientRequest{})
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
	if result.IsError {
		t.Fatalf("legacy search failure changed isError: %+v", result)
	}
	wantOut := textOut{Text: typedErr.Error()}
	var gotOut textOut
	decodeStructured(t, result, &gotOut)
	if gotOut != wantOut {
		t.Fatalf("structured output=%+v, want %+v", gotOut, wantOut)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content len=%d, want 1", len(result.Content))
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]=%T, want *mcp.TextContent", result.Content[0])
	}
	var contentOut textOut
	if err := json.Unmarshal([]byte(textContent.Text), &contentOut); err != nil {
		t.Fatalf("decode text content %q: %v", textContent.Text, err)
	}
	if contentOut != wantOut {
		t.Fatalf("text content=%+v, want %+v", contentOut, wantOut)
	}

	event := findOperationEvent(t, logs.String(), "search_illust")
	if event["level"] != "ERROR" || event["result"] != "error" || event["error_code"] != "" || event["backend"] != "local" || event["status"] != float64(http.StatusBadGateway) {
		t.Fatalf("sanitized hostile typed event=%v", event)
	}
	for _, secret := range []string{codeCanary, backendCanary, "token-secret", "secret.example", "backend-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("MCP log leaked hostile typed metadata %q: %s", secret, logs.String())
		}
	}
}

func TestLegacyDownloadValidationPreservesStructuredResultAndLogsError(t *testing.T) {
	var logs bytes.Buffer
	server := New(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewJSONHandler(&logs, nil)))
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
	wantText := "Error: provide either illust_id (single ID) or illust_ids (list of IDs)."
	assertEmptyDownloadResult(t, result, deliveryLocalPath, wantText)
	event := findOperationEvent(t, logs.String(), "download")
	if event["level"] != "ERROR" || event["result"] != "error" || event["backend"] != "local" || event["error_code"] != "" || event["status"] != float64(0) {
		t.Fatalf("download validation event=%v", event)
	}
}

func TestLegacyRefreshAuthenticationFailurePreservesTextAndLogsTypedError(t *testing.T) {
	typedErr := &sdk.Error{
		Code:      sdk.CodeUnauthorized,
		Operation: sdk.OperationRefresh,
		Backend:   sdk.BackendOAuth,
	}
	var logs bytes.Buffer
	service := application.SDKService{NewClient: func(application.SDKClientRequest) (application.SDKClient, error) {
		return &failingRefreshSDKClient{err: typedErr}, nil
	}}
	server := NewWithSDK(&fakeAPI{}, &fakeDownloads{}, slog.New(slog.NewJSONHandler(&logs, nil)), service, application.SDKClientRequest{})
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

	result := callTool(t, session, "refresh_token", map[string]any{})
	if result.IsError {
		t.Fatalf("legacy refresh failure changed isError: %+v", result)
	}
	var out textOut
	decodeStructured(t, result, &out)
	want := "Error: no refresh token is configured. Use set_refresh_token to set one first."
	if out.Text != want {
		t.Fatalf("refresh output=%q, want %q", out.Text, want)
	}
	event := findOperationEvent(t, logs.String(), "refresh_token")
	if event["level"] != "ERROR" || event["result"] != "error" || event["error_code"] != string(sdk.CodeUnauthorized) || event["backend"] != string(sdk.BackendOAuth) {
		t.Fatalf("refresh authentication event=%v", event)
	}
}

func findOperationEvent(t *testing.T, logs, operation string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		if event["operation"] == operation {
			return event
		}
	}
	t.Fatalf("operation %q event missing from %s", operation, fmt.Sprintf("%q", logs))
	return nil
}
