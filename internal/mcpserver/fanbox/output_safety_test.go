package fanbox_test

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// unsafeOutputPropertyNames mirrors the Pixiv product contract: a FANBOX tool
// output may carry an opaque resource reference, never the session cookie,
// request headers, signed locator or expiry needed to replay a request outside
// the SDK.
//
// The list is intentionally duplicated per product rather than shared. Each
// MCP product owns its own output contract and its own registry construction,
// and a shared test package would couple the two products' test builds for
// twenty lines of table. If a third product appears, converge them then.
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
	"session":         {},
	"fanboxsessid":    {},
	"token":           {},
}

// TestEveryToolOutputSchemaOmitsTransportAndCredentialFields walks the output
// schema of every registered FANBOX tool as an MCP client sees it and rejects
// any property that would hand a client replayable transport or session state.
//
// FANBOX authenticates with a long-lived session cookie, so leaking transport
// metadata here is strictly worse than on the Pixiv side: a signed locator plus
// its headers is enough to fetch paid content outside the SDK.
func TestEveryToolOutputSchemaOmitsTransportAndCredentialFields(t *testing.T) {
	service, _ := fanboxTestService(t, fanboxsdkOKRoundTripper())
	session, closeSession := newFanboxMCPSession(t, service)
	defer closeSession()

	tools := listFanboxTools(t, session)
	if len(tools) == 0 {
		t.Fatal("no tools registered")
	}

	violations := unsafeSchemaProperties(t, tools)
	if len(violations) > 0 {
		t.Fatalf("tool output schemas expose transport or credential fields:\n  %s", strings.Join(violations, "\n  "))
	}
}

// TestUnsafeOutputPropertyIsDetected proves the walk above can fail, so a
// schema shape the walker does not understand cannot silently turn the check
// into a no-op for every tool.
func TestUnsafeOutputPropertyIsDetected(t *testing.T) {
	probe := []*mcp.Tool{{
		Name: "probe_tool",
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"assets": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":       "object",
						"properties": map[string]any{"cookie": map[string]any{"type": "string"}},
					},
				},
			},
		},
	}}

	violations := unsafeSchemaProperties(t, probe)
	if len(violations) != 1 || !strings.Contains(violations[0], "cookie") {
		t.Fatalf("nested unsafe property was not detected, got %v", violations)
	}
}

func listFanboxTools(t *testing.T, session *mcp.ClientSession) []*mcp.Tool {
	t.Helper()
	tools := make([]*mcp.Tool, 0)
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("tools: %v", err)
		}
		tools = append(tools, tool)
	}
	return tools
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
// keeps the check independent of which schema keywords the generator emits.
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
