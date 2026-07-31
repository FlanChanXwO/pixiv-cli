package pixiv

import "testing"

func TestDrawingToolSuggestionRejectsParallelOneEditCandidates(t *testing.T) {
	if suggestion, ok := drawingToolSuggestionFromCatalog("alpla", []string{"alpha", "alpba"}); ok || suggestion != "" {
		t.Fatalf("ambiguous one-edit candidates produced suggestion %q", suggestion)
	}
}
