package timeline

import (
	"bytes"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
)

func TestNewRegistersTimelineLeaves(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	seen := map[string]bool{}
	for _, child := range cmd.Commands() {
		seen[child.Name()] = true
	}
	for _, name := range []string{"following", "latest"} {
		if !seen[name] {
			t.Fatalf("timeline command missing %q", name)
		}
	}
}
