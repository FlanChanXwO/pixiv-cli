package follow

import (
	"bytes"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
)

func TestNewRegistersFollowMutations(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	if len(cmd.Commands()) != 2 || cmd.Commands()[0].Name() != "add" || cmd.Commands()[1].Name() != "remove" {
		t.Fatalf("unexpected follow leaves: %v", cmd.Commands())
	}
}
