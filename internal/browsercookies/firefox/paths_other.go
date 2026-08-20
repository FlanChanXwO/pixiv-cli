//go:build !darwin && !linux && !windows

package firefox

func firefoxDataRoot(home string) string { return "" }
