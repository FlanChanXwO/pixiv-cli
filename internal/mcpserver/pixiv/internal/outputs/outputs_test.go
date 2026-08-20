package outputs

import (
	"errors"
	"testing"
)

func TestErrorReturnsStructuredEmptyEnvelope(t *testing.T) {
	result, out, err := Error(errors.New("upstream unavailable"))
	if err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if !result.IsError || len(out.Records) != 0 || out.Pagination.Page != 1 {
		t.Fatalf("unexpected error envelope: result=%#v out=%#v", result, out)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected one diagnostic content item, got %d", len(result.Content))
	}
}
