package pixiv_test

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// unsafeOutputPropertyNames are JSON property names that would expose request
// transport or credential material to an MCP client. A tool output may carry
// an opaque resource reference, never the headers, cookies, signed locator or
// expiry needed to replay a request outside the SDK.
var unsafeOutputPropertyNames = map[string]struct{}{
	"cookie":          {},
	"cookies":         {},
	"expiry":          {},
	"expires_at":      {},
	"headers":         {},
	"locator":         {},
	"request_headers": {},
	"refresh_token":   {},
	"access_token":    {},
	"token":           {},
}

// TestEveryToolOutputSchemaOmitsTransportAndCredentialFields walks the output
// schema of every registered Pixiv tool as an MCP client sees it and rejects
// any property that would hand a client replayable transport state.
//
// This asserts against the live registry rather than source text, so adding a
// tool, renaming its output type, or moving it to another package cannot slip
// past the check. A tool without an output schema is reported too: an
// unschematized output is exactly the case where an unsafe field would go
// unnoticed.
func TestEveryToolOutputSchemaOmitsTransportAndCredentialFields(t *testing.T) {
	tools := connectAndListTools(t)
	if len(tools) == 0 {
		t.Fatal("no tools registered")
	}

	violations := unsafeSchemaProperties(t, tools)
	if len(violations) > 0 {
		t.Fatalf("tool output schemas expose transport or credential fields:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestUnsafeOutputPropertyIsDetected proves the walk above can fail. Without
// it, a schema shape the walker does not understand would silently turn the
// check into a no-op for every tool.
func TestUnsafeOutputPropertyIsDetected(t *testing.T) {
	probe := []*mcp.Tool{{
		Name: "probe_tool",
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"request_headers": map[string]any{"type": "object"},
						},
					},
				},
			},
		},
	}}

	violations := unsafeSchemaProperties(t, probe)
	if len(violations) != 1 || !strings.Contains(violations[0], "request_headers") {
		t.Fatalf("nested unsafe property was not detected, got %v", violations)
	}
}

func unsafeSchemaProperties(t *testing.T, tools []*mcp.Tool) []string {
	t.Helper()
	violations := make([]string, 0)
	for _, tool := range tools {
		if tool.OutputSchema == nil {
			violations = append(violations, tool.Name+": no output schema")
			continue
		}
		encoded, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatalf("marshal %s output schema: %v", tool.Name, err)
		}
		var schema any
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatalf("decode %s output schema: %v", tool.Name, err)
		}
		for _, path := range unsafeSchemaKeys(schema, "") {
			violations = append(violations, tool.Name+": "+path)
		}
	}
	sort.Strings(violations)
	return violations
}

// unsafeSchemaKeys reports every object key in the decoded schema whose name is
// forbidden, at any depth. Walking raw keys rather than only `properties` maps
// keeps the check independent of which schema keywords the generator emits, so
// a nested `$defs`, `items`, `anyOf` or vendor extension cannot hide a field.
// JSON Schema keywords themselves are never in the forbidden set, so this
// cannot produce a false positive from schema structure alone.
func unsafeSchemaKeys(node any, path string) []string {
	found := make([]string, 0)
	switch value := node.(type) {
	case map[string]any:
		names := make([]string, 0, len(value))
		for name := range value {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			child := path + "/" + name
			if _, unsafe := unsafeOutputPropertyNames[strings.ToLower(name)]; unsafe {
				found = append(found, child)
			}
			found = append(found, unsafeSchemaKeys(value[name], child)...)
		}
	case []any:
		for index, child := range value {
			found = append(found, unsafeSchemaKeys(child, path+"/"+strconv.Itoa(index))...)
		}
	}
	return found
}

func connectAndListTools(t *testing.T) []*mcp.Tool {
	t.Helper()
	server := pixivmcpserver.New(&fakeAPI{}, &fakeDownloads{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = server.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools := make([]*mcp.Tool, 0)
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("tools: %v", err)
		}
		tools = append(tools, tool)
	}
	return tools
}
