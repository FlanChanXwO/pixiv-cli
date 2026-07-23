package config

import (
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/common/constants"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigFilePathUsesUserHomeDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path, err := defaultConfigFilePath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, constants.AppDataDirName, "config.toml"), path)
}
