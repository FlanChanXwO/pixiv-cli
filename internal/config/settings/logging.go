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
	default:
		return value, nil
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
