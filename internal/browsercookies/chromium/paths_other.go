//go:build !darwin && !linux && !windows

package chromium

func browserDataRoot(home string, k kind) string { return "" }
