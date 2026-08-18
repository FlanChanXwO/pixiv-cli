package paths_test

import (
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	"github.com/stretchr/testify/require"
)

func TestApplicationDataPathsPreserveExistingLayout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	directory, err := paths.AppDataDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, paths.AppDataDirName), directory)

	file, err := paths.UserDataFile(paths.AppDataDirName, "config.toml")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, paths.AppDataDirName, "config.toml"), file)

	configPath, err := paths.ConfigFilePath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, paths.AppDataDirName, "config.toml"), configPath)
}

func TestConfigPathOverrideIsReversible(t *testing.T) {
	original, err := paths.ConfigFilePath()
	require.NoError(t, err)
	override := filepath.Join(t.TempDir(), "config.toml")
	restore := paths.SetConfigFilePathForTest(override)
	require.Equal(t, override, mustConfigPath(t))
	restore()
	require.Equal(t, original, mustConfigPath(t))
}

func mustConfigPath(t *testing.T) string {
	t.Helper()
	path, err := paths.ConfigFilePath()
	require.NoError(t, err)
	return path
}
