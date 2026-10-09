package schemas

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestClosedObjectNormalizesNilRequired pins the wire contract that broke
// tools/list: JSON Schema requires "required" to be an array of strings, so a
// Go nil slice has to serialize as [] and never as null.
func TestClosedObjectNormalizesNilRequired(t *testing.T) {
	encoded, err := json.Marshal(ClosedObject(map[string]any{}, nil))
	if err != nil {
		t.Fatalf("marshal ClosedObject: %v", err)
	}

	var decoded struct {
		Required json.RawMessage `json:"required"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode ClosedObject: %v", err)
	}
	if string(decoded.Required) != "[]" {
		t.Fatalf("required = %s, want []", decoded.Required)
	}
}

// TestListWithoutRequiredSerializesAsArray covers the caller shape that
// produced the nil in the first place: List(props) with no variadic required
// arguments yields a nil slice, not an empty one.
func TestListWithoutRequiredSerializesAsArray(t *testing.T) {
	encoded, err := json.Marshal(List(map[string]any{}))
	if err != nil {
		t.Fatalf("marshal List: %v", err)
	}
	if want := `"required":[]`; !strings.Contains(string(encoded), want) {
		t.Fatalf("List() encoded = %s, want it to contain %s", encoded, want)
	}
}

// TestClosedObjectPreservesExplicitRequired proves the normalization above does
// not swallow a real requirement list.
func TestClosedObjectPreservesExplicitRequired(t *testing.T) {
	encoded, err := json.Marshal(ClosedObject(map[string]any{"id": map[string]any{"type": "integer"}}, []string{"id"}))
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
