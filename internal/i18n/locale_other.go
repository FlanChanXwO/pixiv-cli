//go:build !darwin && !windows

package i18n

func nativeLanguage() string { return "en" }
