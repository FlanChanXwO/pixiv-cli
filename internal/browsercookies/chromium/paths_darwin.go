//go:build darwin

package chromium

import "path/filepath"

func browserDataRoot(home string, k kind) string {
	if k == kindEdge {
		return filepath.Join(home, "Library", "Application Support", "Microsoft Edge")
	}
	return filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
}
