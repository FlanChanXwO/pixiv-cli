//go:build windows

package firefox

import "path/filepath"

func firefoxDataRoot(home string) string {
	return filepath.Join(home, "AppData", "Roaming", "Mozilla", "Firefox")
}
