//go:build linux

package firefox

import (
	"os"
	"path/filepath"
)

func firefoxDataRoot(home string) string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "mozilla", "firefox")
}
