package comment

import (
	"bytes"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
)

func TestNewDeclaresCommentInputAndTypeFlags(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	if cmd.Use != "comment ID" {
		t.Fatalf("unexpected comment use: %q", cmd.Use)
	}
	if cmd.Flags().Lookup("type") == nil || cmd.Flags().Lookup("ndjson") == nil {
		t.Fatal("comment command did not register required type/ndjson flags")
	}
}
