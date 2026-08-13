package localstate_test

import (
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	"github.com/stretchr/testify/require"
)

func TestApplicationDataPathsPreserveExistingLayout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	directory, err := localstate.AppDataDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, localstate.AppDataDirName), directory)

	file, err := localstate.UserDataFile(localstate.AppDataDirName, "config.toml")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, localstate.AppDataDirName, "config.toml"), file)

	configPath, err := localstate.ConfigFilePath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, localstate.AppDataDirName, "config.toml"), configPath)
}

func TestConfigPathOverrideIsReversible(t *testing.T) {
	original, err := localstate.ConfigFilePath()
	require.NoError(t, err)
	override := filepath.Join(t.TempDir(), "config.toml")
	restore := localstate.SetConfigFilePathForTest(override)
	require.Equal(t, override, mustConfigPath(t))
	restore()
	require.Equal(t, original, mustConfigPath(t))
}

func mustConfigPath(t *testing.T) string {
	t.Helper()
	path, err := localstate.ConfigFilePath()
	require.NoError(t, err)
	return path
}
