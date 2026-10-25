package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/stretchr/testify/require"
)

func TestConfigurationPriorityAndExplicitEmpty(t *testing.T) {
	for _, tc := range []struct {
		name, body, want, source string
		environment              *string
	}{
		{"default", "", "./downloads", "default", nil},
		{"file", "[download]\npath='./file'", "./file", "file", nil},
		{"environment", "[download]\npath='./file'", "./env", "env", new("./env")},
		{"empty environment", "[download]\npath='./file'", "", "env", new("")},
		{"empty file", "[download]\npath=''", "", "file", nil},
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
			store := settings.DefaultStore()
			path, err := store.Path()
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o600))
			snapshot, err := store.Current()
			require.NoError(t, err)
			value, err := snapshot.Effective(settings.KeyDownloadPath)
			require.NoError(t, err)
			require.Equal(t, tc.source, value.Source)
			require.Equal(t, tc.want, value.Value)
			require.True(t, value.HasValue)
		})
	}
}
