//go:build linux

package chromium

import (
	"os"
	"path/filepath"
)

func browserDataRoot(home string, k kind) string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	if k == kindEdge {
		return filepath.Join(configHome, "microsoft-edge")
	}
	return filepath.Join(configHome, "google-chrome")
}
