package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestUsageErrorExitCode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var output, diagnostics bytes.Buffer
	code := cli.Run([]string{"pixiv", "search", "--unknown-flag"}, strings.NewReader(""), &output, &diagnostics)
	require.Equal(t, 2, code)
	require.Empty(t, output.String())
	require.Contains(t, diagnostics.String(), "unknown option '--unknown-flag'")
}
