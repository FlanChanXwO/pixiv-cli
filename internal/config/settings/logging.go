package settings

import (
	"fmt"
	"strings"
)

func normalizeSpecialString(alias, value string) (string, error) {
	switch alias {
	case "log_level":
		return normalizeLogLevel(value)
	case "log_format":
		return normalizeLogFormat(value)
	case "reverse_search_provider":
		return normalizeReverseSearchProvider(value)
	default:
		return value, nil
	}
}

func normalizeReverseSearchProvider(value string) (string, error) {
	value = strings.TrimSpace(value)
	switch value {
	case "saucenao", "ascii2d-color", "ascii2d-bovw", "all":
		return value, nil
	default:
		return "", fmt.Errorf("reverse_search_provider must be one of: saucenao, ascii2d-color, ascii2d-bovw, all")
	}
}

func normalizeLogLevel(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value != "info" && value != "debug" {
		return "", fmt.Errorf("log_level must be one of: info, debug")
	}
	return value, nil
}

func normalizeLogFormat(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value != "text" && value != "json" {
		return "", fmt.Errorf("log_format must be one of: text, json")
	}
	return value, nil
}
