package series

import (
	"bytes"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
)

func TestNewDeclaresSeriesInputAndTypeFlags(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	if cmd.Use != "series SERIES_ID" {
		t.Fatalf("unexpected series use: %q", cmd.Use)
	}
	if cmd.Flags().Lookup("type") == nil || cmd.Flags().Lookup("ndjson") == nil {
		t.Fatal("series command did not register required type/ndjson flags")
	}
}
