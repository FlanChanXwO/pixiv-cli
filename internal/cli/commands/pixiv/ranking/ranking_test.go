package ranking

import (
	"bytes"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
)

func TestNewDeclaresRankingInputAndOutputFlags(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	if cmd.Use != "ranking" {
		t.Fatalf("unexpected ranking use: %q", cmd.Use)
	}
	for _, name := range []string{"mode", "date", "limit", "page", "ndjson"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("ranking command missing flag %q", name)
		}
	}
}
