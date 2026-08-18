package bookmark

import (
	"bytes"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
)

func TestNewRegistersBookmarkLeaves(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	seen := map[string]bool{}
	for _, child := range cmd.Commands() {
		seen[child.Name()] = true
	}
	for _, name := range []string{"list", "tags", "detail", "add", "remove"} {
		if !seen[name] {
			t.Fatalf("bookmark command missing %q", name)
		}
	}
}
