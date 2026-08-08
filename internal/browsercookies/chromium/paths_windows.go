//go:build windows

package chromium

import "path/filepath"

func browserDataRoot(home string, k kind) string {
	base := filepath.Join(home, "AppData", "Local")
	if k == kindEdge {
		return filepath.Join(base, "Microsoft", "Edge", "User Data")
	}
	return filepath.Join(base, "Google", "Chrome", "User Data")
}
