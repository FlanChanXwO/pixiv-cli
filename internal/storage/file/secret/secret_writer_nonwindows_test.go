//go:build !windows

package secret_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	secret "github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteSecretFilePreservesExistingParentOwnership(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "exports")
	require.NoError(t, os.Mkdir(directory, 0o751))
	before, err := os.Stat(directory)
	require.NoError(t, err)
	beforeSystem, ok := before.Sys().(*syscall.Stat_t)
	require.True(t, ok)

	require.NoError(t, secret.WriteSecretFile(filepath.Join(directory, "auth-export.json"), []byte("secret"), false))
	after, err := os.Stat(directory)
	require.NoError(t, err)
	afterSystem, ok := after.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	assert.Equal(t, beforeSystem.Uid, afterSystem.Uid)
	assert.Equal(t, beforeSystem.Gid, afterSystem.Gid)
}
