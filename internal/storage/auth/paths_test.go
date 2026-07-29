package auth

import (
	"path/filepath"
	"testing"

	constants "github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/stretchr/testify/require"
)

func TestDefaultAuthFilePathUsesUserHomeDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path, err := defaultAuthFilePath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, constants.AppDataDirName, "auth.json"), path)
}
