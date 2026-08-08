package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	configapp "github.com/FlanChanXwO/pixiv-cli/internal/application/config"
	"github.com/stretchr/testify/require"
)

func TestProductionConfigPathSetAndGetPersistParsedValue(t *testing.T) {
	clearRuntimeEnvironment(t)
	configPath := filepath.Join(t.TempDir(), "nested", "config.toml")
	t.Cleanup(config.SetFilePathForTest(configPath))
	service := configapp.ConfigService{Store: configapp.ConfigFileStore{Files: filesystemConfigFiles{}}}

	path, err := service.Path()
	require.NoError(t, err)
	require.Equal(t, configPath, path)
	mutation, err := service.Set("output_json", "true")
	require.NoError(t, err)
	require.Equal(t, "output_json", mutation.Alias)
	require.False(t, mutation.HasOverride)
	value, err := service.Get("output_json")

	require.NoError(t, err)
	require.Equal(t, true, value.Value)
	require.Equal(t, "true", value.Text)
	require.Equal(t, "file", value.Source)
	require.True(t, value.HasValue)
}

func TestProductionConfigReportsEnvironmentOverrideAcrossSetAndUnset(t *testing.T) {
	clearRuntimeEnvironment(t)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	t.Cleanup(config.SetFilePathForTest(configPath))
	service := configapp.ConfigService{Store: configapp.ConfigFileStore{Files: filesystemConfigFiles{}}}
	t.Setenv("DOWNLOAD_PATH", "/environment/path")

	setResult, err := service.Set("download_path", "/file/path")
	require.NoError(t, err)
	require.Equal(t, "download_path", setResult.Alias)
	require.True(t, setResult.HasOverride)
	require.Equal(t, "/environment/path", setResult.EnvOverride)
	effective, err := service.Get("download_path")
	require.NoError(t, err)
	require.Equal(t, "/environment/path", effective.Value)
	require.Equal(t, "env", effective.Source)

	require.NoError(t, os.Unsetenv("DOWNLOAD_PATH"))
	persisted, err := service.Get("download_path")
	require.NoError(t, err)
	require.Equal(t, "/file/path", persisted.Value)
	require.Equal(t, "file", persisted.Source)

	t.Setenv("DOWNLOAD_PATH", "/environment/path")
	unsetResult, err := service.Unset("download_path")
	require.NoError(t, err)
	require.Equal(t, "download_path", unsetResult.Alias)
	require.True(t, unsetResult.HasOverride)
	require.Equal(t, "/environment/path", unsetResult.EnvOverride)
	require.NoError(t, os.Unsetenv("DOWNLOAD_PATH"))
	defaulted, err := service.Get("download_path")
	require.NoError(t, err)
	require.Equal(t, config.DefaultDownloadPath, defaulted.Value)
	require.Equal(t, "default", defaulted.Source)
}

func TestProductionConfigRejectsInvalidInputWithoutChangingEffectiveValue(t *testing.T) {
	clearRuntimeEnvironment(t)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	t.Cleanup(config.SetFilePathForTest(configPath))
	service := configapp.ConfigService{Store: configapp.ConfigFileStore{Files: filesystemConfigFiles{}}}

	mutation, err := service.Set("output_json", "sometimes")

	require.Empty(t, mutation)
	require.ErrorContains(t, err, "expects bool value")
	value, getErr := service.Get("output_json")
	require.NoError(t, getErr)
	require.Equal(t, false, value.Value)
	require.Equal(t, "default", value.Source)
}

func TestProductionConfigPropagatesMalformedFileErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(service configapp.ConfigService) error
	}{
		{name: "get", call: func(service configapp.ConfigService) error {
			_, err := service.Get("output_json")
			return err
		}},
		{name: "set", call: func(service configapp.ConfigService) error {
			_, err := service.Set("output_json", "true")
			return err
		}},
		{name: "unset", call: func(service configapp.ConfigService) error {
			_, err := service.Unset("output_json")
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearRuntimeEnvironment(t)
			configPath := filepath.Join(t.TempDir(), "config.toml")
			t.Cleanup(config.SetFilePathForTest(configPath))
			require.NoError(t, config.WritePrivateFile(configPath, []byte("[output\njson = broken")))

			err := test.call(configapp.ConfigService{Store: configapp.ConfigFileStore{Files: filesystemConfigFiles{}}})

			require.Error(t, err)
		})
	}
}
