package chromium

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestBrowserDataRootMatchesSupportedPlatformLayout(t *testing.T) {
	home := filepath.FromSlash("/fixture-home")
	switch runtime.GOOS {
	case "darwin":
		if got := browserDataRoot(home, kindChrome); got != filepath.Join(home, "Library", "Application Support", "Google", "Chrome") {
			t.Fatalf("Chrome root = %q", got)
		}
		if got := browserDataRoot(home, kindEdge); got != filepath.Join(home, "Library", "Application Support", "Microsoft Edge") {
			t.Fatalf("Edge root = %q", got)
		}
	case "linux":
		configHome := filepath.Join(home, "xdg-config")
		t.Setenv("XDG_CONFIG_HOME", configHome)
		if got := browserDataRoot(home, kindChrome); got != filepath.Join(configHome, "google-chrome") {
			t.Fatalf("Chrome root = %q", got)
		}
		if got := browserDataRoot(home, kindEdge); got != filepath.Join(configHome, "microsoft-edge") {
			t.Fatalf("Edge root = %q", got)
		}
	case "windows":
		if got := browserDataRoot(home, kindChrome); got != filepath.Join(home, "AppData", "Local", "Google", "Chrome", "User Data") {
			t.Fatalf("Chrome root = %q", got)
		}
		if got := browserDataRoot(home, kindEdge); got != filepath.Join(home, "AppData", "Local", "Microsoft", "Edge", "User Data") {
			t.Fatalf("Edge root = %q", got)
		}
	default:
		if got := browserDataRoot(home, kindChrome); got != "" {
			t.Fatalf("unsupported-platform Chrome root = %q, want empty", got)
		}
	}
}
