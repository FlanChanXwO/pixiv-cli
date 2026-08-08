package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDownloadSettingsExposeDirectoryTemplateAndRequestInterval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[download]\ndirectory_template = \"{author}/{date}\"\n[network]\nrequest_interval = \"2s\"\n"), 0o600))
	state, err := LoadSettingsStateAt(path)
	require.NoError(t, err)
	runtime, err := state.Runtime()
	require.NoError(t, err)
	require.Equal(t, "{author}/{date}", runtime.DirectoryTemplate)
	require.Equal(t, 2*time.Second, runtime.RequestInterval)

	t.Setenv("PIXIV_REQUEST_INTERVAL", "3s")
	runtime, err = state.Runtime()
	require.NoError(t, err)
	require.Equal(t, 3*time.Second, runtime.RequestInterval)
}

func TestRequestIntervalRejectsNegativeDuration(t *testing.T) {
	_, _, err := ParseSettingInput("request_interval", "-1s")
	require.EqualError(t, err, "request_interval must not be negative")

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[network]\nrequest_interval = \"-1s\"\n"), 0o600))
	state, err := LoadSettingsStateAt(path)
	require.NoError(t, err)
	_, err = state.Runtime()
	require.EqualError(t, err, "request_interval must not be negative")
}
