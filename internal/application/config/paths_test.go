package config

import (
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/filesystem"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigFilePathUsesUserHomeDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path, err := defaultConfigFilePath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, filesystem.AppDataDirName, "config.toml"), path)
}
