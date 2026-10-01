package settings

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/creachadair/tomledit/parser"
)

func settingStringValue(value SettingValue) string {
	if !value.HasValue {
		return ""
	}
	text, _ := value.Value.(string)
	return text
}

func coerceSettingValue(spec SettingSpec, raw any, source string) (SettingValue, error) {
	switch spec.Kind {
	case settingString:
		text, ok := normalizeStringValue(raw)
		if !ok {
			return SettingValue{}, fmt.Errorf("%s expects string, got %T", spec.Alias, raw)
		}
		text, err := normalizeSpecialString(spec.Alias, text)
		if err != nil {
			return SettingValue{}, err
		}
		if text == "" && !spec.HasDefault && source != "default" {
			return SettingValue{Source: source}, nil
		}
		return SettingValue{Value: text, Text: text, Source: source, HasValue: true}, nil
	case settingBool:
		value, err := normalizeBoolValue(raw)
		if err != nil {
			return SettingValue{}, fmt.Errorf("%s: %w", spec.Alias, err)
		}
		return SettingValue{Value: value, Text: strconv.FormatBool(value), Source: source, HasValue: true}, nil
	case settingDuration:
		value, err := normalizeDurationValue(raw)
		if err != nil {
			return SettingValue{}, fmt.Errorf("%s: %w", spec.Alias, err)
		}
		if spec.Alias == "request_interval" && value < 0 {
			return SettingValue{}, errors.New("request_interval must not be negative")
		}
		return SettingValue{Value: value, Text: value.String(), Source: source, HasValue: true}, nil
	default:
		return SettingValue{}, fmt.Errorf("unsupported setting kind %q", spec.Kind)
	}
}

func normalizeStringValue(raw any) (string, bool) {
	switch value := raw.(type) {
	case string:
		return value, true
	case fmt.Stringer:
		return value.String(), true
	case nil:
		return "", false
	default:
		return fmt.Sprint(value), true
	}
}

func normalizeBoolValue(raw any) (bool, error) {
	switch value := raw.(type) {
	case bool:
		return value, nil
	case string:
		return strconv.ParseBool(strings.TrimSpace(value))
	default:
		return false, fmt.Errorf("expects bool, got %T", raw)
	}
}

func normalizeDurationValue(raw any) (time.Duration, error) {
	switch value := raw.(type) {
	case time.Duration:
		return value, nil
	case string:
		return time.ParseDuration(strings.TrimSpace(value))
	default:
		return 0, fmt.Errorf("expects duration string, got %T", raw)
	}
}

func ParseSettingInput(alias, raw string) (SettingValue, parser.Value, error) {
	spec, ok := SettingSpecByAlias(alias)
	if !ok {
		return SettingValue{}, parser.Value{}, fmt.Errorf("unknown config key %q", alias)
	}
	if spec.Removed {
		return SettingValue{}, parser.Value{}, RemovedSettingError(alias)
	}
	raw = strings.TrimSpace(raw)
	switch spec.Kind {
	case settingString:
		normalized, err := normalizeSpecialString(alias, raw)
		if err != nil {
			return SettingValue{}, parser.Value{}, err
		}
		raw = normalized
		value, err := parser.ParseValue(strconv.Quote(raw))
		if err != nil {
			return SettingValue{}, parser.Value{}, err
		}
		return SettingValue{Value: raw, Text: raw, Source: "cli", HasValue: true}, value, nil
	case settingBool:
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return SettingValue{}, parser.Value{}, fmt.Errorf("expects bool value: %w", err)
		}
		value, err := parser.ParseValue(strconv.FormatBool(parsed))
		if err != nil {
			return SettingValue{}, parser.Value{}, err
		}
		return SettingValue{Value: parsed, Text: strconv.FormatBool(parsed), Source: "cli", HasValue: true}, value, nil
	case settingDuration:
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return SettingValue{}, parser.Value{}, fmt.Errorf("expects duration value: %w", err)
		}
		if spec.Alias == "request_interval" && parsed < 0 {
			return SettingValue{}, parser.Value{}, errors.New("request_interval must not be negative")
		}
		normalized := parsed.String()
		value, err := parser.ParseValue(strconv.Quote(normalized))
		if err != nil {
			return SettingValue{}, parser.Value{}, err
		}
		return SettingValue{Value: parsed, Text: normalized, Source: "cli", HasValue: true}, value, nil
	default:
		return SettingValue{}, parser.Value{}, fmt.Errorf("unsupported setting kind %q", spec.Kind)
	}
}

// 以下规范化函数属于"类型解析/规范化/领域校验"职责：它们把用户输入收敛为
// 领域允许的字面量，非法值返回明确错误而不是被静默接受。

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
