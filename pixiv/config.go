package pixiv

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/config"
)

// ConfigKey 是 SDK 支持读取或写入的本地配置键。
type ConfigKey string

const (
	ConfigKeyDownloadPath          ConfigKey = "download_path"
	ConfigKeyFilenameTemplate      ConfigKey = "filename_template"
	ConfigKeyDirectoryTemplate     ConfigKey = "directory_template"
	ConfigKeyHTTPSProxy            ConfigKey = "https_proxy"
	ConfigKeyRequestInterval       ConfigKey = "request_interval"
	ConfigKeyWebFallbackEnabled    ConfigKey = "web_fallback_enabled"
	ConfigKeyUpdateCheckEnabled    ConfigKey = "update_check_enabled"
	ConfigKeyOutputJSON            ConfigKey = "output_json"
	ConfigKeyLoginOpenBrowser      ConfigKey = "login_open_browser"
	ConfigKeyLoginTimeout          ConfigKey = "login_timeout"
	ConfigKeyLoginUseAfterLogin    ConfigKey = "login_use_after_login"
	ConfigKeyLoginRelayPublicURL   ConfigKey = "login_relay_public_url"
	ConfigKeyLoginRelayListenAddr  ConfigKey = "login_relay_listen_addr"
	ConfigKeyLoginRelayTLSCertFile ConfigKey = "login_relay_tls_cert_file"
	ConfigKeyLoginRelayTLSKeyFile  ConfigKey = "login_relay_tls_key_file"
	ConfigKeyPremiumStatusCacheTTL ConfigKey = "premium_status_cache_ttl"
)

// ConfigValueKind 描述 ConfigValue 或 ConfigInput 保存的 Go 值类型。
type ConfigValueKind string

const (
	ConfigValueKindString   ConfigValueKind = "string"
	ConfigValueKindBool     ConfigValueKind = "bool"
	ConfigValueKindDuration ConfigValueKind = "duration"
)

// ConfigSource 描述一个有效配置值来自环境、文件、默认值还是未设置状态。
type ConfigSource string

const (
	ConfigSourceEnvironment ConfigSource = "env"
	ConfigSourceFile        ConfigSource = "file"
	ConfigSourceDefault     ConfigSource = "default"
	ConfigSourceUnset       ConfigSource = "unset"
)

// ConfigValue 是可安全返回给 SDK 调用方的强类型配置视图。Kind 决定哪个值字段有效。
// Redacted 为真时不会包含敏感原文。
type ConfigValue struct {
	Key      ConfigKey       `json:"key"`
	Kind     ConfigValueKind `json:"kind"`
	String   string          `json:"string,omitempty"`
	Bool     bool            `json:"bool,omitempty"`
	Duration time.Duration   `json:"duration,omitempty"`
	Source   ConfigSource    `json:"source"`
	HasValue bool            `json:"has_value"`
	Redacted bool            `json:"redacted,omitempty"`
}

// ConfigInput 是写入一个强类型配置值的输入。它只能由本包的构造器或
// ParseConfigInput 创建，避免调用方给 bool key 写入字符串。
type ConfigInput struct {
	kind     ConfigValueKind
	stringV  string
	boolV    bool
	duration time.Duration
}

// StringConfigInput 构造字符串配置输入。
func StringConfigInput(value string) ConfigInput {
	return ConfigInput{kind: ConfigValueKindString, stringV: value}
}

// BoolConfigInput 构造布尔配置输入。
func BoolConfigInput(value bool) ConfigInput {
	return ConfigInput{kind: ConfigValueKindBool, boolV: value}
}

// DurationConfigInput 构造 duration 配置输入。
func DurationConfigInput(value time.Duration) ConfigInput {
	return ConfigInput{kind: ConfigValueKindDuration, duration: value}
}

// ParseConfigKey 把 CLI/MCP 等文本边界的 alias 映射为强类型 ConfigKey。
func ParseConfigKey(alias string) (ConfigKey, error) {
	key := ConfigKey(alias)
	if _, ok := config.SettingSpecByAlias(string(key)); !ok {
		return "", errors.New("config key is invalid")
	}
	return key, nil
}

// ParseConfigInput 只供文本边界把一个已知 key 的字符串值转为强类型输入。
func ParseConfigInput(key ConfigKey, raw string) (ConfigInput, error) {
	if err := validateConfigKey(key); err != nil {
		return ConfigInput{}, err
	}
	value, _, err := config.ParseSettingInput(string(key), raw)
	if err != nil {
		return ConfigInput{}, errors.New("config value is invalid")
	}
	return configInputFromValue(value)
}

func configInputFromValue(value config.SettingValue) (ConfigInput, error) {
	if !value.HasValue {
		return ConfigInput{}, errors.New("config value is required")
	}
	switch typed := value.Value.(type) {
	case string:
		return StringConfigInput(typed), nil
	case bool:
		return BoolConfigInput(typed), nil
	case time.Duration:
		return DurationConfigInput(typed), nil
	default:
		return ConfigInput{}, errors.New("config value type is unsupported")
	}
}

func validateConfigKey(key ConfigKey) error {
	if _, ok := config.SettingSpecByAlias(string(key)); !ok {
		return errors.New("config key is invalid")
	}
	return nil
}

func configInputText(key ConfigKey, input ConfigInput) (string, error) {
	spec, ok := config.SettingSpecByAlias(string(key))
	if !ok {
		return "", errors.New("config key is invalid")
	}
	switch input.kind {
	case ConfigValueKindString:
		if spec.Kind != "string" {
			return "", errors.New("config value type does not match key")
		}
		return strconv.Quote(input.stringV), nil
	case ConfigValueKindBool:
		if spec.Kind != "bool" {
			return "", errors.New("config value type does not match key")
		}
		return strconv.FormatBool(input.boolV), nil
	case ConfigValueKindDuration:
		if spec.Kind != "duration" {
			return "", errors.New("config value type does not match key")
		}
		return strconv.Quote(input.duration.String()), nil
	default:
		return "", errors.New("config value type is invalid")
	}
}

func publicConfigValue(key ConfigKey, value config.SettingValue) (ConfigValue, error) {
	result := ConfigValue{Key: key, Source: ConfigSource(value.Source), HasValue: value.HasValue}
	spec, ok := config.SettingSpecByAlias(string(key))
	if !ok {
		return ConfigValue{}, errors.New("config key is invalid")
	}
	if spec.Sensitive {
		result.Kind = ConfigValueKindString
		result.Redacted = value.HasValue
		return result, nil
	}
	if !value.HasValue {
		switch spec.Kind {
		case "string":
			result.Kind = ConfigValueKindString
		case "bool":
			result.Kind = ConfigValueKindBool
		case "duration":
			result.Kind = ConfigValueKindDuration
		}
		return result, nil
	}
	switch typed := value.Value.(type) {
	case string:
		result.Kind, result.String = ConfigValueKindString, typed
	case bool:
		result.Kind, result.Bool = ConfigValueKindBool, typed
	case time.Duration:
		result.Kind, result.Duration = ConfigValueKindDuration, typed
	default:
		return ConfigValue{}, fmt.Errorf("unsupported config value type %T", value.Value)
	}
	return result, nil
}
