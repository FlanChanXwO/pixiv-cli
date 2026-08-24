package follow

import (
	"bytes"
	"context"
	"strings"
	"testing"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistersFollowMutations(t *testing.T) {
	cmd := New(deps.Data{Input: strings.NewReader(""), Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}, UsageError: func(err error) error { return err }})
	if len(cmd.Commands()) != 2 || cmd.Commands()[0].Name() != "add" || cmd.Commands()[1].Name() != "remove" {
		t.Fatalf("unexpected follow leaves: %v", cmd.Commands())
	}
}

func TestFollowRecordTypesRemainUserOnly(t *testing.T) {
	assert.Equal(t, map[string]struct{}{"user": {}}, userRecordTypes)

	called := false
	var diagnostics bytes.Buffer
	err := pipeline.ConsumeActionRecords(
		context.Background(),
		strings.NewReader(`{"id":"42","type":"artwork","url":"https://www.pixiv.net/artworks/42"}`+"\n"),
		&diagnostics, "follow_add", "fail-fast", userRecordTypes,
		func(context.Context, int64) error { called = true; return nil },
		func(err error) error { return err },
	)
	require.Error(t, err)
	assert.Contains(t, diagnostics.String(), `"code":"unsupported_type"`)
	assert.False(t, called)
}
