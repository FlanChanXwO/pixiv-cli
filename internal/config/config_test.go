package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

func TestConfigurationPriorityAndExplicitEmpty(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		environment      *string
	}{
		{"default", "", "./downloads", nil},
		{"file", "[download]\npath='./file'", "./file", nil},
		{"environment", "[download]\npath='./file'", "./env", new("./env")},
		{"empty environment", "[download]\npath='./file'", "", new("")},
		{"empty file", "[download]\npath=''", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("DOWNLOAD_PATH", "")
			require.NoError(t, os.Unsetenv("DOWNLOAD_PATH"))
			if tc.environment != nil {
				t.Setenv("DOWNLOAD_PATH", *tc.environment)
			}
			store := config.Store{Files: secretfile.ConfigFiles{}}
			path, err := store.Path()
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))
			value, present, err := store.Get("download.path")
			require.NoError(t, err)
			require.Equal(t, tc.want, value)
			require.True(t, present)
		})
	}
}

func TestDefaultTemplate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store := config.Store{Files: secretfile.ConfigFiles{}}
	require.NoError(t, store.EnsureDefaultConfigFile())
	path, err := store.Path()
	require.NoError(t, err)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var decoded config.Config
	require.NoError(t, toml.Unmarshal(body, &decoded))
	require.Equal(t, config.Default(), decoded)
}
