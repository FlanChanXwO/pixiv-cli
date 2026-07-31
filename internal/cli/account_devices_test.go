package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthCommandDoesNotRegisterDevices(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "auth", "devices", "list"}, bytes.NewReader(nil), &stdout, &stderr)
	require.Equal(t, 1, code)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "usage: pixiv auth <command>")
	require.NotContains(t, stderr.String(), "devices list")
}
