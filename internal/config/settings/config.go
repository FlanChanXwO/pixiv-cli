package settings

import (
	"time"

	"github.com/knadh/koanf/v2"
)

type settingKind string

const (
	settingString   settingKind = "string"
	settingBool     settingKind = "bool"
	settingDuration settingKind = "duration"
	settingInteger  settingKind = "integer"
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

	// RequestInterval 具有运行时默认值，但不写入精简初始文件。
	RequestInterval time.Duration `config:"network.request_interval" alias:"request_interval" env:"PIXIV_REQUEST_INTERVAL" default:"0s" cli:"true"`

	// LogLevel 与 LogFormat 具有默认值并进入精简配置。
	LogLevel  string `config:"logging.level" alias:"log_level" env:"PIXIV_LOG_LEVEL" default:"info" cli:"true" example:"true"`
	LogFormat string `config:"logging.format" alias:"log_format" env:"PIXIV_LOG_FORMAT" default:"text" cli:"true" example:"true"`

	// PixivNetwork / FanboxNetwork / ReverseSearchNetwork 以及两个 FlareSolverr 组
	// 是高级可选配置，只在用户显式写入 TOML 表时生效，且不经过扁平 config set。
	// 路径来自声明；存在性、严格类型和 solver 启用条件仍由 snapshot.go 领域读取负责。
	PixivNetwork              PixivNetworkConfig   `config:"pixiv.network"`
	FanboxNetwork             ServiceNetworkConfig `config:"fanbox.network"`
	ReverseSearchNetwork      ServiceNetworkConfig `config:"reverse_search.network"`
	FanboxFlareSolverr        *FlareSolverrConfig  `config:"fanbox.flaresolverr"`
	ReverseSearchFlareSolverr *FlareSolverrConfig  `config:"reverse_search.flaresolverr"`

	// 布尔开关：除 account_pool 外都不由 config 命令管理。
	OutputJSON         bool `config:"output.json" alias:"output_json" default:"false" example:"true"`
	UpdateCheckEnabled bool `config:"update.check_enabled" alias:"update_check_enabled" default:"true" example:"true"`
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

	// AccountPool 的静态路径来自组前缀；严格类型和策略校验仍由领域读取负责。
	AccountPool AccountPoolConfig `config:"account_pool"`
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
	ProxyURL  OptionalString `config:"proxy_url" json:"proxy_url"`
	UserAgent OptionalString `config:"user_agent" json:"user_agent"`
}

// PixivNetworkConfig 是 Pixiv 服务级网络的**窄类型**：它只有 proxy_url。
//
// Pixiv 与 FANBOX/反搜的网络字段并不相同：Pixiv 不接受服务级 user_agent。之前用
// 共享结构体 + `includeUserAgent` 程序分支来维持这个差异，容易在重构中被误删。
// 用独立类型表达后，`pixiv.network.user_agent` 在类型层面就无法表达，也无法被绑定，
// 因此不会在重构中意外变成新的用户可配置能力。
type PixivNetworkConfig struct {
	ProxyURL OptionalString `config:"proxy_url" json:"proxy_url"`
}

// FlareSolverrConfig describes the optional external challenge-recovery
// service. It is nil when the whole table is absent, which keeps the default
// runtime free of any solver dependency.
type FlareSolverrConfig struct {
	URL      string `config:"url" json:"url"`
	ProxyURL string `config:"proxy_url" json:"proxy_url,omitempty"`
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
	Enabled  bool                `config:"enabled" alias:"account_pool_enabled" default:"false" cli:"true"`
	Strategy AccountPoolStrategy `config:"strategy" alias:"account_pool_strategy" default:"round_robin" cli:"true"`
}

// defaultAccountSelection 的路径独立派生；UID 不加入 Runtime 的字段遍历。
// 数值有效性由 auth.go 的按需入口检查，不因加载普通配置提前报错。
type defaultAccountSelection struct {
	Pixiv  defaultAccount `config:"pixiv.auth"`
	Fanbox defaultAccount `config:"fanbox.auth"`
}

type defaultAccount struct {
	UserID int64 `config:"default_user_id"`
}
