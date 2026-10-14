package schemas_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
)

// TestListWithoutRequiredArgumentsSerializesRequiredAsArray pins the wire
// contract that broke tools/list. List forwards its variadic parameter to
// ClosedObject, and calling it with no required arguments yields a nil slice,
// which encodes as JSON null. JSON Schema requires required to be an array of
// strings, so a nil slice has to serialize as [].
func TestListWithoutRequiredArgumentsSerializesRequiredAsArray(t *testing.T) {
	encoded, err := json.Marshal(schemas.List(map[string]any{}))
	if err != nil {
		t.Fatalf("marshal List: %v", err)
	}
	if want := `"required":[]`; !strings.Contains(string(encoded), want) {
		t.Fatalf("List() encoded = %s, want it to contain %s", encoded, want)
	}
}

// TestClosedObjectPreservesExplicitRequired proves the nil normalization does
// not swallow a real requirement list.
func TestClosedObjectPreservesExplicitRequired(t *testing.T) {
	encoded, err := json.Marshal(schemas.ClosedObject(map[string]any{"id": map[string]any{"type": "integer"}}, []string{"id"}))
	if err != nil {
		t.Fatalf("marshal ClosedObject: %v", err)
	}

	var decoded struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode ClosedObject: %v", err)
	}
	if len(decoded.Required) != 1 || decoded.Required[0] != "id" {
		t.Fatalf("required = %v, want [id]", decoded.Required)
	}
}
