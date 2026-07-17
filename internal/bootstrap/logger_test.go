package bootstrap

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewApplicationLoggerUsesDiscardWhenConfigIsMalformed(t *testing.T) {
	clearRuntimeEnvironment(t)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	t.Cleanup(config.SetFilePathForTest(configPath))
	require.NoError(t, config.WritePrivateFile(configPath, []byte("[logging\nlevel = broken")))
	var output bytes.Buffer
	global := slog.Default()

	logger, err := NewApplicationLogger(&output)
	require.NoError(t, err)
	logger.Error("must remain local")

	require.Empty(t, output.String())
	require.Same(t, global, slog.Default())
}

func TestNewApplicationLoggerRejectsInvalidExplicitLoggingSettings(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		wantError string
	}{
		{name: "level", body: "[logging]\nlevel = \"verbose\"\nformat = \"text\"\n", wantError: "log_level must be one of"},
		{name: "format", body: "[logging]\nlevel = \"info\"\nformat = \"xml\"\n", wantError: "log_format must be text or json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearRuntimeEnvironment(t)
			configPath := filepath.Join(t.TempDir(), "config.toml")
			t.Cleanup(config.SetFilePathForTest(configPath))
			require.NoError(t, config.WritePrivateFile(configPath, []byte(test.body)))
			global := slog.Default()

			logger, err := NewApplicationLogger(&bytes.Buffer{})

			require.Nil(t, logger)
			require.ErrorContains(t, err, test.wantError)
			require.Same(t, global, slog.Default())
		})
	}
}

func TestNewApplicationLoggerWritesConfiguredFormatsOnlyToProvidedWriter(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			clearRuntimeEnvironment(t)
			configPath := filepath.Join(t.TempDir(), "config.toml")
			t.Cleanup(config.SetFilePathForTest(configPath))
			body := "[logging]\nlevel = \"info\"\nformat = \"" + format + "\"\n"
			require.NoError(t, config.WritePrivateFile(configPath, []byte(body)))
			var output bytes.Buffer
			global := slog.Default()

			logger, err := NewApplicationLogger(&output)
			require.NoError(t, err)
			logger.Info("bootstrap logger probe", "format", format)

			require.Same(t, global, slog.Default())
			require.Contains(t, output.String(), "bootstrap logger probe")
			if format == "json" {
				var event map[string]any
				require.NoError(t, json.Unmarshal(output.Bytes(), &event))
				require.Equal(t, "INFO", event["level"])
				require.Equal(t, format, event["format"])
			} else {
				require.Contains(t, output.String(), "level=INFO")
				require.Contains(t, output.String(), "format=text")
			}
		})
	}
}
