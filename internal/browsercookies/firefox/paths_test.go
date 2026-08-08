package firefox

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestFirefoxDataRootMatchesSupportedPlatformLayout(t *testing.T) {
	home := filepath.FromSlash("/fixture-home")
	switch runtime.GOOS {
	case "darwin":
		if got := firefoxDataRoot(home); got != filepath.Join(home, "Library", "Application Support", "Firefox") {
			t.Fatalf("Firefox root = %q", got)
		}
	case "linux":
		configHome := filepath.Join(home, "xdg-config")
		t.Setenv("XDG_CONFIG_HOME", configHome)
		if got := firefoxDataRoot(home); got != filepath.Join(configHome, "mozilla", "firefox") {
			t.Fatalf("Firefox root = %q", got)
		}
	case "windows":
		if got := firefoxDataRoot(home); got != filepath.Join(home, "AppData", "Roaming", "Mozilla", "Firefox") {
			t.Fatalf("Firefox root = %q", got)
		}
	default:
		if got := firefoxDataRoot(home); got != "" {
			t.Fatalf("unsupported-platform Firefox root = %q, want empty", got)
		}
	}
}
