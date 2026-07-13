package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestRuntimeLoggerUsesEnvironmentOverFileAndEmitsJSON(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	if err := WritePrivateFile(path, []byte("[logging]\nlevel = 'error'\nformat = 'text'\n")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXIV_LOG_LEVEL", "trace")
	t.Setenv("PIXIV_LOG_FORMAT", "json")
	state, err := LoadSettingsStateAt(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := state.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.LogLevel != "trace" || runtime.LogFormat != "json" {
		t.Fatalf("runtime logger = %+v", runtime)
	}
	var output bytes.Buffer
	logger, err := NewLogger(&output, runtime)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("trace-visible", "component", "test")
	if !strings.Contains(output.String(), `"msg":"trace-visible"`) {
		t.Fatalf("trace JSON log missing: %q", output.String())
	}
}

func TestRuntimeLoggerRejectsInvalidConfiguredValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "level", body: "[logging]\nlevel = 'loud'\n", want: "log_level"},
		{name: "format", body: "[logging]\nformat = 'yaml'\n", want: "log_format"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir() + "/config.toml"
			if err := WritePrivateFile(path, []byte(tc.body)); err != nil {
				t.Fatal(err)
			}
			state, err := LoadSettingsStateAt(path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = state.Runtime()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Runtime error = %v, want %q", err, tc.want)
			}
		})
	}
}
