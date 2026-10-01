package settings

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
	"github.com/creachadair/tomledit/transform"
	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/v2"
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
	// DefaultInFile 表示该默认值是否进入首次生成的精简 config.toml。
	DefaultInFile bool
	// CLIManaged 表示该键是否由 pixiv config get/set/unset 暴露。
	CLIManaged bool
	// Removed 表示该键已随版本删除，只保留迁移墓碑。旧配置仍显式包含它时返回
	// removed_setting；pixiv config unset 允许用户清理。墓碑不得驱动运行时分支。
	Removed bool
}

type SettingValue struct {
	Value    any
	Text     string
	Source   string
	HasValue bool
}

type Snapshot struct {
	file *koanf.Koanf
	env  map[string]snapshotEnvValue
}

type snapshotEnvValue struct {
	value   string
	present bool
}

type RuntimeConfig struct {
	// DownloadPath 是作品下载根目录；未配置时使用相对当前工作目录的路径。
	DownloadPath string `config:"download.path" alias:"download_path" env:"DOWNLOAD_PATH" default:"./downloads" cli:"true" example:"true"`

	// FilenameTemplate 是下载文件名模板，进入首次生成的精简配置。
	FilenameTemplate string `config:"download.filename_template" alias:"filename_template" env:"FILENAME_TEMPLATE" default:"{author} - {title}_{id}" cli:"true" example:"true"`

	// DirectoryTemplate 是可选下载子目录模板；没有默认值，因此不进入精简配置。
	DirectoryTemplate string `config:"download.directory_template" alias:"directory_template" env:"DIRECTORY_TEMPLATE" cli:"true"`

	// HTTPSProxy 保留小写环境变量优先于大写环境变量的现有顺序，且没有默认值。
	HTTPSProxy string `config:"network.https_proxy" alias:"https_proxy" env:"https_proxy,HTTPS_PROXY" cli:"true"`

	// LogLevel 与 LogFormat 具有默认值并进入精简配置。
	LogLevel  string `config:"logging.level" alias:"log_level" env:"PIXIV_LOG_LEVEL" default:"info" cli:"true" example:"true"`
	LogFormat string `config:"logging.format" alias:"log_format" env:"PIXIV_LOG_FORMAT" default:"text" cli:"true" example:"true"`

	// PixivNetwork / FanboxNetwork / ReverseSearchNetwork 以及两个 FlareSolverr 组
	// 是高级可选配置，只在用户显式写入 TOML 表时生效，且不经过扁平 config set。
	// 它们的绑定规则保留在 snapshot.go 中（服务级代理的"缺失 vs 显式空串"语义
	// 与 Pixiv 不接受 user_agent 的差异无法用扁平的 config 标签表达）。
	PixivNetwork              ServiceNetworkConfig `config:"-"`
	FanboxNetwork             ServiceNetworkConfig `config:"-"`
	ReverseSearchNetwork      ServiceNetworkConfig `config:"-"`
	FanboxFlareSolverr        *FlareSolverrConfig  `config:"-"`
	ReverseSearchFlareSolverr *FlareSolverrConfig  `config:"-"`

	// RequestInterval 具有运行时默认值，但不写入精简初始文件。
	RequestInterval time.Duration `config:"network.request_interval" alias:"request_interval" env:"PIXIV_REQUEST_INTERVAL" default:"0s" cli:"true"`

	// 布尔开关：除 account_pool 外都不由 config 命令管理。
	UpdateCheckEnabled bool `config:"update.check_enabled" alias:"update_check_enabled" default:"true" example:"true"`
	OutputJSON         bool `config:"output.json" alias:"output_json" default:"false" example:"true"`
	LoginOpenBrowser   bool `config:"login.open_browser" alias:"login_open_browser" default:"true" example:"true"`
	LoginUseAfterLogin bool `config:"login.use_after_login" alias:"login_use_after_login" default:"false" example:"true"`

	// 反搜设置。SauceNAOAPIKey 只能经过现有私密输入链路写入，不进入初始配置或公开输出。
	ReverseSearchProvider  string `config:"reverse_search.provider" alias:"reverse_search_provider" default:"saucenao" cli:"true" example:"true"`
	ReverseSearchPixivOnly bool   `config:"reverse_search.pixiv_only" alias:"reverse_search_pixiv_only" default:"true" cli:"true" example:"true"`
	SauceNAOAPIKey         string `config:"reverse_search.saucenao_api_key" alias:"saucenao_api_key" env:"SAUCENAO_API_KEY" cli:"true" secret:"true"`

	// LoginRelay* 描述本次运行时创建的跨机器浏览器中继。历史 secret/target
	// 配置项仍可留在私有配置文件中，但不会载入 runtime，避免恢复旧 client relay。
	LoginRelayPublicURL   string `config:"login.relay_public_url" alias:"login_relay_public_url"`
	LoginRelayListenAddr  string `config:"login.relay_listen_addr" alias:"login_relay_listen_addr"`
	LoginRelayTLSCertFile string `config:"login.relay_tls_cert_file" alias:"login_relay_tls_cert_file"`
	LoginRelayTLSKeyFile  string `config:"login.relay_tls_key_file" alias:"login_relay_tls_key_file"`

	// AccountPool 只能在 config.toml 的 [account_pool] 表中手工维护，避免普通
	// config set 的扁平字符串接口误写账号白名单。
	AccountPool AccountPoolConfig `config:"-"`
}

// OptionalString preserves the difference between an absent advanced TOML
// key and an explicitly configured empty string. That distinction is required
// for service-scoped proxy routing: an empty service value means direct access
// instead of inheriting the global fallback.
type OptionalString struct {
	Present bool   `json:"present"`
	Value   string `json:"value,omitempty"`
}

// ServiceNetworkConfig contains only service-local network settings. These
// values are not aliases and are intentionally absent from the generated
// baseline config until a user writes the advanced TOML table.
type ServiceNetworkConfig struct {
	ProxyURL  OptionalString `json:"proxy_url"`
	UserAgent OptionalString `json:"user_agent"`
}

// FlareSolverrConfig describes the optional external challenge-recovery
// service. It is nil when the whole table is absent, which keeps the default
// runtime free of any solver dependency.
type FlareSolverrConfig struct {
	URL      string `json:"url"`
	ProxyURL string `json:"proxy_url,omitempty"`
}

type AccountPoolStrategy string

const (
	AccountPoolStrategyRoundRobin AccountPoolStrategy = "round_robin"
	AccountPoolStrategyRandom     AccountPoolStrategy = "random"
)

// AccountPoolConfig 描述内容读取和作品下载是否启用数据库账号调度及其策略。
// UID、冻结时间和 marker 不进入 config.toml。字段标签驱动声明；[account_pool]
// 表仍直接读取，策略枚举由 accountPool() 的领域校验负责。
type AccountPoolConfig struct {
	Enabled  bool                `config:"account_pool.enabled" alias:"account_pool_enabled" default:"false" cli:"true"`
	Strategy AccountPoolStrategy `config:"account_pool.strategy" alias:"account_pool_strategy" default:"round_robin" cli:"true"`
}

// SettingSpecByAlias 返回 alias 对应的 spec。已移除键仍可被查询，以便 config unset
// 执行清理、config get/set 返回 removed_setting。元数据完全来自字段标签声明。
func SettingSpecByAlias(alias string) (SettingSpec, bool) {
	for _, entry := range mustSettingSpecs() {
		if entry.spec.Alias == alias {
			return entry.spec, true
		}
	}
	return SettingSpec{}, false
}

// mustSettingSpecs 返回派生 schema；声明错误在此以 panic 暴露，因为它是编程错误，
// 只能在首次使用时才能通过 public API 观察到。
func mustSettingSpecs() []settingSpecFromTags {
	derived, err := settingSpecsFromTags()
	if err != nil {
		panic(fmt.Sprintf("invalid configuration schema: %v", err))
	}
	return derived
}

// ErrRemovedSetting 表示该配置键已随版本删除。旧配置仍显式包含它时返回；用户
// 通过 `pixiv config unset` 清理后不再出现。
var ErrRemovedSetting = errors.New("removed_setting")

// RemovedSettingError 构造一个同时能被 errors.Is 匹配 ErrRemovedSetting 的错误，
// 明确指导用户如何清理旧配置。已移除键在显式写入配置时对 runtime 生效，不设置
// 默认值，也不进入可写别名集合。
func RemovedSettingError(alias string) error {
	return fmt.Errorf("%w: config key %q was removed; clear it with `pixiv config unset %s`", ErrRemovedSetting, alias, alias)
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
	derived := mustSettingSpecs()
	keys := make([]string, 0, len(derived))
	for _, entry := range derived {
		if entry.spec.Removed {
			continue
		}
		keys = append(keys, entry.spec.Alias)
	}
	slices.Sort(keys)
	return keys
}

// CLISettingAliases 返回由 pixiv config 命令管理的非移除配置键。
func CLISettingAliases() []string {
	derived := mustSettingSpecs()
	keys := make([]string, 0, len(derived))
	for _, entry := range derived {
		if entry.spec.Removed || !entry.spec.CLIManaged {
			continue
		}
		keys = append(keys, entry.spec.Alias)
	}
	slices.Sort(keys)
	return keys
}

// EnvValue 返回 spec 对应别名按 env 标签声明顺序解析到的环境变量值。
//
// 契约：
//   - 顺序即优先级，返回第一个**存在**的变量；某个变量存在但为空也算命中，
//     绝不继续回退到后续变量（这是服务级代理"显式直连"语义依赖的行为）。
//   - 未声明 env 标签的别名返回 ("", false)。
//
// 来源完全来自字段标签，不再维护独立的 alias switch。
func EnvValue(spec SettingSpec) (string, bool) {
	for _, name := range envNamesFor(spec.Alias) {
		if value, present := os.LookupEnv(name); present {
			return value, true
		}
	}
	return "", false
}

func LoadSnapshot() (Snapshot, error) {
	store := defaultFileStore{}
	path, err := store.Path()
	if err != nil {
		return Snapshot{}, err
	}
	return LoadSnapshotAtWithFileStore(path, store)
}

// LoadSnapshotAt 从明确给定的路径加载配置。SDK 需要它避免改动
// 包级测试路径或依赖调用进程的隐式当前配置位置。
func LoadSnapshotAt(path string) (Snapshot, error) {
	return LoadSnapshotAtWithFileStore(path, defaultFileStore{})
}

// LoadSnapshotAtWithFileStore 从明确路径和注入的文件端口加载配置。它
// 让 composition root 可以把 platform/file mechanism 适配传入 schema owner。
func LoadSnapshotAtWithFileStore(path string, store FileStore) (Snapshot, error) {
	store, err := requireFileStore(store)
	if err != nil {
		return Snapshot{}, err
	}
	fileState := koanf.New(".")
	if err := loadConfigFileInto(fileState, path, store.ReadFile); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{file: fileState, env: captureEnvironment()}, nil
}

// captureEnvironment 固定一次 snapshot 的环境 precedence。若 Effective 每次重新
// 读取 os.Environ，命令运行中修改环境会让同一个 snapshot 产生不同结果。
func captureEnvironment() map[string]snapshotEnvValue {
	values := make(map[string]snapshotEnvValue)
	for _, entry := range mustSettingSpecs() {
		if raw, present := EnvValue(entry.spec); present {
			values[entry.spec.Alias] = snapshotEnvValue{value: raw, present: true}
		}
	}
	return values
}

type rawFileProvider struct{ body []byte }

func (p rawFileProvider) ReadBytes() ([]byte, error) { return p.body, nil }

func (rawFileProvider) Read() (map[string]any, error) {
	return nil, errors.New("raw configuration provider does not support parsed reads")
}

func loadConfigFileInto(target *koanf.Koanf, path string, readFile func(string) ([]byte, error)) error {
	body, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	return target.Load(rawFileProvider{body: body}, toml.Parser())
}

func (s Snapshot) Effective(alias string) (SettingValue, error) {
	spec, ok := SettingSpecByAlias(alias)
	if !ok {
		return SettingValue{}, fmt.Errorf("unknown config key %q", alias)
	}
	if spec.Removed {
		if s.file.Exists(spec.KoanfKey) {
			return SettingValue{}, RemovedSettingError(alias)
		}
		return SettingValue{Source: "unset"}, nil
	}
	if raw, ok := s.env[spec.Alias]; ok && raw.present {
		return coerceSettingValue(spec, raw.value, "env")
	}
	if s.file.Exists(spec.KoanfKey) {
		return coerceSettingValue(spec, s.file.Get(spec.KoanfKey), "file")
	}
	if spec.HasDefault {
		return coerceSettingValue(spec, spec.Default, "default")
	}
	return SettingValue{Source: "unset"}, nil
}

func (s Snapshot) Runtime() (RuntimeConfig, error) {
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
	logLevel, err := s.Effective("log_level")
	if err != nil {
		return RuntimeConfig{}, err
	}
	logFormat, err := s.Effective("log_format")
	if err != nil {
		return RuntimeConfig{}, err
	}
	outputJSON, err := s.Effective("output_json")
	if err != nil {
		return RuntimeConfig{}, err
	}
	// web_fallback_enabled 是 v1 迁移墓碑：旧配置仍显式包含它时在此失败并要求清理，
	// 但不再驱动任何运行时分支。
	if _, err := s.Effective("web_fallback_enabled"); err != nil {
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
	reverseSearchProvider, err := s.Effective("reverse_search_provider")
	if err != nil {
		return RuntimeConfig{}, err
	}
	reverseSearchPixivOnly, err := s.Effective("reverse_search_pixiv_only")
	if err != nil {
		return RuntimeConfig{}, err
	}
	sauceNAOAPIKey, err := s.Effective("saucenao_api_key")
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
	normalizedLogLevel, err := normalizeLogLevel(settingStringValue(logLevel))
	if err != nil {
		return RuntimeConfig{}, err
	}
	normalizedLogFormat, err := normalizeLogFormat(settingStringValue(logFormat))
	if err != nil {
		return RuntimeConfig{}, err
	}
	accountPool, err := s.accountPool()
	if err != nil {
		return RuntimeConfig{}, err
	}
	pixivNetwork, err := s.serviceNetwork("pixiv.network", false)
	if err != nil {
		return RuntimeConfig{}, err
	}
	fanboxNetwork, err := s.serviceNetwork("fanbox.network", true)
	if err != nil {
		return RuntimeConfig{}, err
	}
	reverseSearchNetwork, err := s.serviceNetwork("reverse_search.network", true)
	if err != nil {
		return RuntimeConfig{}, err
	}
	flareSolverr, err := s.flareSolverr()
	if err != nil {
		return RuntimeConfig{}, err
	}
	reverseSearchFlareSolverr, err := s.flareSolverrAt("reverse_search.flaresolverr")
	if err != nil {
		return RuntimeConfig{}, err
	}
	cfg := RuntimeConfig{
		DownloadPath:              downloadPath.Value.(string),
		FilenameTemplate:          filenameTemplate.Value.(string),
		DirectoryTemplate:         settingStringValue(directoryTemplate),
		HTTPSProxy:                "",
		LogLevel:                  normalizedLogLevel,
		LogFormat:                 normalizedLogFormat,
		PixivNetwork:              pixivNetwork,
		FanboxNetwork:             fanboxNetwork,
		ReverseSearchNetwork:      reverseSearchNetwork,
		FanboxFlareSolverr:        flareSolverr,
		ReverseSearchFlareSolverr: reverseSearchFlareSolverr,
		UpdateCheckEnabled:        updateCheckEnabled.Value.(bool),
		OutputJSON:                outputJSON.Value.(bool),
		LoginOpenBrowser:          loginOpenBrowser.Value.(bool),
		LoginUseAfterLogin:        loginUseAfterLogin.Value.(bool),
		ReverseSearchProvider:     reverseSearchProvider.Value.(string),
		ReverseSearchPixivOnly:    reverseSearchPixivOnly.Value.(bool),
		SauceNAOAPIKey:            settingStringValue(sauceNAOAPIKey),
		LoginRelayPublicURL:       settingStringValue(loginRelayPublicURL),
		LoginRelayListenAddr:      settingStringValue(loginRelayListenAddr),
		LoginRelayTLSCertFile:     settingStringValue(loginRelayTLSCertFile),
		LoginRelayTLSKeyFile:      settingStringValue(loginRelayTLSKeyFile),
		AccountPool:               accountPool,
	}
	if requestInterval.HasValue {
		cfg.RequestInterval = requestInterval.Value.(time.Duration)
	}
	if httpsProxy.HasValue {
		cfg.HTTPSProxy = httpsProxy.Value.(string)
	}
	return cfg, nil
}

func (s Snapshot) serviceNetwork(prefix string, includeUserAgent bool) (ServiceNetworkConfig, error) {
	proxyURL, err := s.optionalString(prefix + ".proxy_url")
	if err != nil {
		return ServiceNetworkConfig{}, err
	}
	network := ServiceNetworkConfig{ProxyURL: proxyURL}
	if includeUserAgent {
		userAgent, err := s.optionalString(prefix + ".user_agent")
		if err != nil {
			return ServiceNetworkConfig{}, err
		}
		network.UserAgent = userAgent
	}
	return network, nil
}

func (s Snapshot) optionalString(path string) (OptionalString, error) {
	if s.file == nil || !s.file.Exists(path) {
		return OptionalString{}, nil
	}
	raw := s.file.Get(path)
	value, ok := raw.(string)
	if !ok {
		return OptionalString{}, fmt.Errorf("%s must be a string", path)
	}
	return OptionalString{Present: true, Value: value}, nil
}

func (s Snapshot) flareSolverr() (*FlareSolverrConfig, error) {
	return s.flareSolverrAt("fanbox.flaresolverr")
}

func (s Snapshot) flareSolverrAt(prefix string) (*FlareSolverrConfig, error) {
	urlValue, err := s.optionalString(prefix + ".url")
	if err != nil {
		return nil, err
	}
	proxyValue, err := s.optionalString(prefix + ".proxy_url")
	if err != nil {
		return nil, err
	}
	if !urlValue.Present && !proxyValue.Present {
		return nil, nil
	}
	if !urlValue.Present || strings.TrimSpace(urlValue.Value) == "" {
		return nil, fmt.Errorf("%s.url must be set when %s is configured", prefix, prefix)
	}
	return &FlareSolverrConfig{URL: urlValue.Value, ProxyURL: proxyValue.Value}, nil
}

func (s Snapshot) accountPool() (AccountPoolConfig, error) {
	pool := AccountPoolConfig{Strategy: AccountPoolStrategyRoundRobin}
	if _, err := s.Effective("account_pool_accounts"); err != nil {
		return AccountPoolConfig{}, err
	}
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
	return pool, nil
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

func loadConfigDocumentWithFileStore(path string, store FileStore) (*tomledit.Document, error) {
	store, err := requireFileStore(store)
	if err != nil {
		return nil, err
	}
	body, err := store.ReadFile(path)
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

func saveConfigDocumentWithFileStore(path string, doc *tomledit.Document, store FileStore) error {
	store, err := requireFileStore(store)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tomledit.Format(&buf, doc); err != nil {
		return err
	}
	return store.WritePrivateFile(path, buf.Bytes())
}

func SetConfigValue(path, alias string, value parser.Value) error {
	return SetConfigValueWithFileStore(path, alias, value, defaultFileStore{})
}

// SetConfigValueWithFileStore 在注入的文件端口上执行稀疏配置写回。
func SetConfigValueWithFileStore(path, alias string, value parser.Value, store FileStore) error {
	store, err := requireFileStore(store)
	if err != nil {
		return err
	}
	spec, ok := SettingSpecByAlias(alias)
	if !ok {
		return fmt.Errorf("unknown config key %q", alias)
	}
	if spec.Removed {
		return RemovedSettingError(alias)
	}
	doc, err := loadConfigDocumentWithFileStore(path, store)
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
	return saveConfigDocumentWithFileStore(path, doc, store)
}

func UnsetConfigValue(path, alias string) (bool, error) {
	return UnsetConfigValueWithFileStore(path, alias, defaultFileStore{})
}

// UnsetConfigValueWithFileStore 在注入的文件端口上删除一个配置键。
func UnsetConfigValueWithFileStore(path, alias string, store FileStore) (bool, error) {
	store, err := requireFileStore(store)
	if err != nil {
		return false, err
	}
	spec, ok := SettingSpecByAlias(alias)
	if !ok {
		return false, fmt.Errorf("unknown config key %q", alias)
	}
	doc, err := loadConfigDocumentWithFileStore(path, store)
	if err != nil {
		return false, err
	}
	entry := doc.First(append(spec.Table, spec.Key)...)
	if entry == nil {
		return false, saveConfigDocumentWithFileStore(path, doc, store)
	}
	removed := entry.Remove()
	if sectionEntry := transform.FindTable(doc, spec.Table...); sectionEntry != nil && len(sectionEntry.Section.Items) == 0 {
		sectionEntry.Remove()
	}
	return removed, saveConfigDocumentWithFileStore(path, doc, store)
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
