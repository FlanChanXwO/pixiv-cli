package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/credentials"
	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
	"github.com/creachadair/tomledit/transform"
	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	DefaultDownloadPath     = "./downloads"
	DefaultFilenameTemplate = "{author} - {title}_{id}"
)

type settingKind string

const (
	settingString   settingKind = "string"
	settingBool     settingKind = "bool"
	settingDuration settingKind = "duration"
)

type SettingSpec struct {
	Alias    string
	KoanfKey string
	Table    []string
	Key      string
	Kind     settingKind
	// Sensitive 表示该值可用于认证或中继授权。它可以被写入私有 config.toml，
	// 但绝不能通过 config get、日志或错误消息回显。
	Sensitive  bool
	HasDefault bool
	Default    any
}

type SettingValue struct {
	Value    any
	Text     string
	Source   string
	HasValue bool
}

type SettingsState struct {
	file *koanf.Koanf
}

type RuntimeConfig struct {
	DownloadPath       string
	FilenameTemplate   string
	DirectoryTemplate  string
	HTTPSProxy         string
	RequestInterval    time.Duration
	WebFallbackEnabled bool
	UpdateCheckEnabled bool
	OutputJSON         bool
	LoginOpenBrowser   bool
	LoginUseAfterLogin bool
	// LoginRelay* 描述本次运行时创建的跨机器浏览器中继。历史 secret/target
	// 配置项仍可留在私有配置文件中，但不会载入 runtime，避免恢复旧 client relay。
	LoginRelayPublicURL   string
	LoginRelayListenAddr  string
	LoginRelayTLSCertFile string
	LoginRelayTLSKeyFile  string
	// AccountPool 只能在 config.toml 的 [account_pool] 表中手工维护，避免普通
	// config set 的扁平字符串接口误写账号白名单。
	AccountPool AccountPoolConfig
}

type AccountPoolStrategy string

const (
	AccountPoolStrategyRoundRobin AccountPoolStrategy = "round_robin"
	AccountPoolStrategyRandom     AccountPoolStrategy = "random"
)

// AccountPoolConfig 描述内容读取和作品下载可使用的本地受管账号白名单。Accounts
// 为空时使用 auth.json 中全部账号的存储顺序；非空时才是显式白名单。
// 账号 token 仍只存在 auth.json，配置与状态文件均不保存凭据。
type AccountPoolConfig struct {
	Enabled  bool
	Accounts []int64
	Strategy AccountPoolStrategy
}

var settingSpecs = []SettingSpec{
	{Alias: "download_path", KoanfKey: "download.path", Table: []string{"download"}, Key: "path", Kind: settingString, HasDefault: true, Default: DefaultDownloadPath},
	{Alias: "filename_template", KoanfKey: "download.filename_template", Table: []string{"download"}, Key: "filename_template", Kind: settingString, HasDefault: true, Default: DefaultFilenameTemplate},
	{Alias: "directory_template", KoanfKey: "download.directory_template", Table: []string{"download"}, Key: "directory_template", Kind: settingString},
	{Alias: "https_proxy", KoanfKey: "network.https_proxy", Table: []string{"network"}, Key: "https_proxy", Kind: settingString},
	{Alias: "request_interval", KoanfKey: "network.request_interval", Table: []string{"network"}, Key: "request_interval", Kind: settingDuration, HasDefault: true, Default: time.Duration(0)},
	{Alias: "web_fallback_enabled", KoanfKey: "web.fallback_enabled", Table: []string{"web"}, Key: "fallback_enabled", Kind: settingBool, HasDefault: true, Default: true},
	{Alias: "update_check_enabled", KoanfKey: "update.check_enabled", Table: []string{"update"}, Key: "check_enabled", Kind: settingBool, HasDefault: true, Default: true},
	{Alias: "output_json", KoanfKey: "output.json", Table: []string{"output"}, Key: "json", Kind: settingBool, HasDefault: true, Default: false},
	{Alias: "login_open_browser", KoanfKey: "login.open_browser", Table: []string{"login"}, Key: "open_browser", Kind: settingBool, HasDefault: true, Default: true},
	{Alias: "login_use_after_login", KoanfKey: "login.use_after_login", Table: []string{"login"}, Key: "use_after_login", Kind: settingBool, HasDefault: true, Default: false},
	{Alias: "login_relay_public_url", KoanfKey: "login.relay_public_url", Table: []string{"login"}, Key: "relay_public_url", Kind: settingString},
	{Alias: "login_relay_listen_addr", KoanfKey: "login.relay_listen_addr", Table: []string{"login"}, Key: "relay_listen_addr", Kind: settingString},
	{Alias: "login_relay_tls_cert_file", KoanfKey: "login.relay_tls_cert_file", Table: []string{"login"}, Key: "relay_tls_cert_file", Kind: settingString},
	{Alias: "login_relay_tls_key_file", KoanfKey: "login.relay_tls_key_file", Table: []string{"login"}, Key: "relay_tls_key_file", Kind: settingString},
}

func SettingSpecByAlias(alias string) (SettingSpec, bool) {
	for _, spec := range settingSpecs {
		if spec.Alias == alias {
			return spec, true
		}
	}
	return SettingSpec{}, false
}

// IsSensitiveSetting 标记禁止通过公开配置查询回显的凭据型值。
func IsSensitiveSetting(alias string) bool {
	spec, ok := SettingSpecByAlias(alias)
	return ok && spec.Sensitive
}

// PublicSettingText 返回可安全进入 CLI、SDK JSON 与日志边界的配置值。
func PublicSettingText(alias, text string) string {
	if IsSensitiveSetting(alias) && text != "" {
		return "<redacted>"
	}
	return text
}

func ValidSettingAliases() []string {
	keys := make([]string, 0, len(settingSpecs))
	for _, spec := range settingSpecs {
		keys = append(keys, spec.Alias)
	}
	slices.Sort(keys)
	return keys
}

// RefreshTokenFromEnv 读取 App API refresh token。Cookie 形态在此处即被拒绝，
// 不能继续流入默认 SDK 的 OAuth 请求。
func RefreshTokenFromEnv() (string, error) {
	return credentials.ValidateRefreshTokenInput(os.Getenv("PIXIV_REFRESH_TOKEN"))
}

func EnvValue(spec SettingSpec) (string, bool) {
	switch spec.Alias {
	case "download_path":
		return envLookup("DOWNLOAD_PATH")
	case "filename_template":
		return envLookup("FILENAME_TEMPLATE")
	case "directory_template":
		return envLookup("DIRECTORY_TEMPLATE")
	case "request_interval":
		return envLookup("PIXIV_REQUEST_INTERVAL")
	case "https_proxy":
		if value, ok := envLookup("https_proxy"); ok {
			return value, true
		}
		return envLookup("HTTPS_PROXY")
	default:
		return "", false
	}
}

func envLookup(name string) (string, bool) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", false
	}
	return value, true
}

func LoadSettingsState() (SettingsState, error) {
	path, err := ConfigFilePath()
	if err != nil {
		return SettingsState{}, err
	}
	return LoadSettingsStateAt(path)
}

// LoadSettingsStateAt 从明确给定的路径加载配置。SDK 需要它避免改动
// 包级测试路径或依赖调用进程的隐式当前配置位置。
func LoadSettingsStateAt(path string) (SettingsState, error) {
	fileState := koanf.New(".")
	if err := loadConfigFileInto(fileState, path); err != nil {
		return SettingsState{}, err
	}
	return SettingsState{file: fileState}, nil
}

func loadConfigFileInto(target *koanf.Koanf, path string) error {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	return target.Load(file.Provider(path), toml.Parser())
}

func (s SettingsState) Effective(alias string) (SettingValue, error) {
	spec, ok := SettingSpecByAlias(alias)
	if !ok {
		return SettingValue{}, fmt.Errorf("unknown config key %q", alias)
	}
	if raw, ok := EnvValue(spec); ok {
		return coerceSettingValue(spec, raw, "env")
	}
	if s.file.Exists(spec.KoanfKey) {
		return coerceSettingValue(spec, s.file.Get(spec.KoanfKey), "file")
	}
	if spec.HasDefault {
		return coerceSettingValue(spec, spec.Default, "default")
	}
	return SettingValue{Source: "unset"}, nil
}

func (s SettingsState) Runtime() (RuntimeConfig, error) {
	downloadPath, err := s.Effective("download_path")
	if err != nil {
		return RuntimeConfig{}, err
	}
	filenameTemplate, err := s.Effective("filename_template")
	if err != nil {
		return RuntimeConfig{}, err
	}
	directoryTemplate, err := s.Effective("directory_template")
	if err != nil {
		return RuntimeConfig{}, err
	}
	httpsProxy, err := s.Effective("https_proxy")
	if err != nil {
		return RuntimeConfig{}, err
	}
	requestInterval, err := s.Effective("request_interval")
	if err != nil {
		return RuntimeConfig{}, err
	}
	outputJSON, err := s.Effective("output_json")
	if err != nil {
		return RuntimeConfig{}, err
	}
	webFallbackEnabled, err := s.Effective("web_fallback_enabled")
	if err != nil {
		return RuntimeConfig{}, err
	}
	updateCheckEnabled, err := s.Effective("update_check_enabled")
	if err != nil {
		return RuntimeConfig{}, err
	}
	loginOpenBrowser, err := s.Effective("login_open_browser")
	if err != nil {
		return RuntimeConfig{}, err
	}
	loginUseAfterLogin, err := s.Effective("login_use_after_login")
	if err != nil {
		return RuntimeConfig{}, err
	}
	loginRelayPublicURL, err := s.Effective("login_relay_public_url")
	if err != nil {
		return RuntimeConfig{}, err
	}
	loginRelayListenAddr, err := s.Effective("login_relay_listen_addr")
	if err != nil {
		return RuntimeConfig{}, err
	}
	loginRelayTLSCertFile, err := s.Effective("login_relay_tls_cert_file")
	if err != nil {
		return RuntimeConfig{}, err
	}
	loginRelayTLSKeyFile, err := s.Effective("login_relay_tls_key_file")
	if err != nil {
		return RuntimeConfig{}, err
	}
	accountPool, err := s.accountPool()
	if err != nil {
		return RuntimeConfig{}, err
	}
	cfg := RuntimeConfig{
		DownloadPath:          downloadPath.Value.(string),
		FilenameTemplate:      filenameTemplate.Value.(string),
		DirectoryTemplate:     settingStringValue(directoryTemplate),
		HTTPSProxy:            "",
		WebFallbackEnabled:    webFallbackEnabled.Value.(bool),
		UpdateCheckEnabled:    updateCheckEnabled.Value.(bool),
		OutputJSON:            outputJSON.Value.(bool),
		LoginOpenBrowser:      loginOpenBrowser.Value.(bool),
		LoginUseAfterLogin:    loginUseAfterLogin.Value.(bool),
		LoginRelayPublicURL:   settingStringValue(loginRelayPublicURL),
		LoginRelayListenAddr:  settingStringValue(loginRelayListenAddr),
		LoginRelayTLSCertFile: settingStringValue(loginRelayTLSCertFile),
		LoginRelayTLSKeyFile:  settingStringValue(loginRelayTLSKeyFile),
		AccountPool:           accountPool,
	}
	if requestInterval.HasValue {
		cfg.RequestInterval = requestInterval.Value.(time.Duration)
	}
	if httpsProxy.HasValue {
		cfg.HTTPSProxy = httpsProxy.Value.(string)
	}
	return cfg, nil
}

func (s SettingsState) accountPool() (AccountPoolConfig, error) {
	pool := AccountPoolConfig{Strategy: AccountPoolStrategyRoundRobin}
	if s.file == nil || !s.file.Exists("account_pool") {
		return pool, nil
	}
	if raw := s.file.Get("account_pool.enabled"); raw != nil {
		enabled, ok := raw.(bool)
		if !ok {
			return AccountPoolConfig{}, errors.New("account_pool.enabled must be a boolean")
		}
		pool.Enabled = enabled
	}
	if raw := s.file.Get("account_pool.strategy"); raw != nil {
		value, ok := raw.(string)
		if !ok {
			return AccountPoolConfig{}, errors.New("account_pool.strategy must be one of: round_robin, random")
		}
		pool.Strategy = AccountPoolStrategy(strings.TrimSpace(value))
	}
	switch pool.Strategy {
	case AccountPoolStrategyRoundRobin, AccountPoolStrategyRandom:
	default:
		return AccountPoolConfig{}, errors.New("account_pool.strategy must be one of: round_robin, random")
	}
	if raw := s.file.Get("account_pool.accounts"); raw != nil {
		accounts, err := accountPoolUIDs(raw)
		if err != nil {
			return AccountPoolConfig{}, err
		}
		pool.Accounts = accounts
	}
	if !pool.Enabled {
		return pool, nil
	}
	seen := make(map[int64]struct{}, len(pool.Accounts))
	for _, userID := range pool.Accounts {
		if userID <= 0 {
			return AccountPoolConfig{}, errors.New("account_pool.accounts must contain only positive UIDs")
		}
		if _, exists := seen[userID]; exists {
			return AccountPoolConfig{}, errors.New("account_pool.accounts must not contain duplicate UIDs")
		}
		seen[userID] = struct{}{}
	}
	return pool, nil
}

func accountPoolUIDs(raw any) ([]int64, error) {
	values, ok := raw.([]any)
	if !ok {
		return nil, errors.New("account_pool.accounts must be an array of UIDs")
	}
	accounts := make([]int64, 0, len(values))
	for _, value := range values {
		switch number := value.(type) {
		case int64:
			accounts = append(accounts, number)
		case int:
			accounts = append(accounts, int64(number))
		default:
			return nil, errors.New("account_pool.accounts must be an array of UIDs")
		}
	}
	return accounts, nil
}

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
	raw = strings.TrimSpace(raw)
	switch spec.Kind {
	case settingString:
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

func loadConfigDocument(path string) (*tomledit.Document, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &tomledit.Document{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return &tomledit.Document{}, nil
	}
	return tomledit.Parse(bytes.NewReader(body))
}

func saveConfigDocument(path string, doc *tomledit.Document) error {
	var buf bytes.Buffer
	if err := tomledit.Format(&buf, doc); err != nil {
		return err
	}
	return WritePrivateFile(path, buf.Bytes())
}

func SetConfigValue(path, alias string, value parser.Value) error {
	spec, ok := SettingSpecByAlias(alias)
	if !ok {
		return fmt.Errorf("unknown config key %q", alias)
	}
	doc, err := loadConfigDocument(path)
	if err != nil {
		return err
	}
	section := ensureConfigSection(doc, spec.Table)
	inserted := transform.InsertMapping(section, &parser.KeyValue{
		Name:  parser.Key{spec.Key},
		Value: value,
	}, true)
	if !inserted {
		return fmt.Errorf("failed to update config key %q", alias)
	}
	return saveConfigDocument(path, doc)
}

func UnsetConfigValue(path, alias string) (bool, error) {
	spec, ok := SettingSpecByAlias(alias)
	if !ok {
		return false, fmt.Errorf("unknown config key %q", alias)
	}
	doc, err := loadConfigDocument(path)
	if err != nil {
		return false, err
	}
	entry := doc.First(append(spec.Table, spec.Key)...)
	if entry == nil {
		return false, saveConfigDocument(path, doc)
	}
	removed := entry.Remove()
	if sectionEntry := transform.FindTable(doc, spec.Table...); sectionEntry != nil && len(sectionEntry.Section.Items) == 0 {
		sectionEntry.Remove()
	}
	return removed, saveConfigDocument(path, doc)
}

func ensureConfigSection(doc *tomledit.Document, name []string) *tomledit.Section {
	if entry := transform.FindTable(doc, name...); entry != nil {
		return entry.Section
	}
	section := &tomledit.Section{
		Heading: &parser.Heading{Name: parser.Key(name)},
	}
	doc.Sections = append(doc.Sections, section)
	return section
}
