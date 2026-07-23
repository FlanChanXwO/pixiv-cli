package config

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRuntimeLoggerDefaultsToWarn(t *testing.T) {
	clearEnvironmentForTest(t, "PIXIV_LOG_LEVEL")

	state, err := LoadSettingsStateAt(t.TempDir() + "/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := state.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.LogLevel != "warn" {
		t.Fatalf("default runtime log level = %q, want warn", runtime.LogLevel)
	}

	var output bytes.Buffer
	logger, err := NewLogger(&output, runtime)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("ordinary-operation")
	if output.Len() != 0 {
		t.Fatalf("default logger emitted INFO diagnostic: %q", output.String())
	}
}

func TestRuntimeLoggerUsesEnvironmentOverFileAndEmitsText(t *testing.T) {
	path := t.TempDir() + "/config.toml"
	if err := WritePrivateFile(path, []byte("[logging]\nlevel = 'error'\n")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXIV_LOG_LEVEL", "trace")
	state, err := LoadSettingsStateAt(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := state.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.LogLevel != "trace" {
		t.Fatalf("runtime logger = %+v", runtime)
	}
	var output bytes.Buffer
	logger, err := NewLogger(&output, runtime)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("trace-visible", "component", "test")
	if !strings.Contains(output.String(), "msg=trace-visible") || strings.Contains(output.String(), `{"msg"`) {
		t.Fatalf("trace text log missing: %q", output.String())
	}
}

func TestRuntimeLoggerKeepsExplicitInfoDiagnostics(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		clearEnvironmentForTest(t, "PIXIV_LOG_LEVEL")
		path := t.TempDir() + "/config.toml"
		if err := WritePrivateFile(path, []byte("[logging]\nlevel = 'info'\n")); err != nil {
			t.Fatal(err)
		}
		assertInfoDiagnostic(t, path)
	})
	t.Run("environment", func(t *testing.T) {
		path := t.TempDir() + "/config.toml"
		if err := WritePrivateFile(path, []byte("[logging]\nlevel = 'warn'\n")); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PIXIV_LOG_LEVEL", "info")
		assertInfoDiagnostic(t, path)
	})
}

func assertInfoDiagnostic(t *testing.T, path string) {
	t.Helper()
	state, err := LoadSettingsStateAt(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := state.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.LogLevel != "info" {
		t.Fatalf("runtime log level = %q, want info", runtime.LogLevel)
	}
	var output bytes.Buffer
	logger, err := NewLogger(&output, runtime)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("explicit-info")
	if !strings.Contains(output.String(), "explicit-info") {
		t.Fatalf("explicit INFO diagnostic missing: %q", output.String())
	}
}

func clearEnvironmentForTest(t *testing.T, key string) {
	t.Helper()
	value, hadValue := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func TestRuntimeLoggerRejectsInvalidConfiguredValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "level", body: "[logging]\nlevel = 'loud'\n", want: "log_level"},
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

func TestLogFormatIsNotAConfigSetting(t *testing.T) {
	t.Parallel()
	if _, ok := SettingSpecByAlias("log_format"); ok {
		t.Fatal("log_format must not remain a configurable JSON/text switch")
	}
}
