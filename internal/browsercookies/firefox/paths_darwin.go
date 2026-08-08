//go:build darwin

package firefox

import "path/filepath"

func firefoxDataRoot(home string) string {
	return filepath.Join(home, "Library", "Application Support", "Firefox")
}
