package settings

import "time"

type settingKind string

const (
	settingString   settingKind = "string"
	settingBool     settingKind = "bool"
	settingDuration settingKind = "duration"
	settingInteger  settingKind = "integer"
)

// SettingSpec 汇集一个配置键的声明元数据，由 struct 字段标签派生。
type SettingSpec struct {
	Alias    Key
	TOMLPath string
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
}

// SettingValue 给出某个键的有效值及其来源；HasValue 为 false 时该键未设置。
type SettingValue struct {
	Value    any
	Text     string
	Source   string
	HasValue bool
}

// Snapshot 给出一次执行内的不可变配置视图，由 [Store.Current] 生成。
type Snapshot struct {
	file map[string]any
	env  map[Key]string
}

// RuntimeConfig 表示文件、环境变量与默认值合并后的配置结构；字段标签声明路径、别名和默认值。
type RuntimeConfig struct {
	MCPGalleryAnimationTempCapacityMiB int64 `config:"mcp.gallery_animation_temp_capacity_mib" alias:"mcp_gallery_animation_temp_capacity_mib" default:"1024" cli:"true"`
	MCPGalleryCacheCapacityMiB         int64 `config:"mcp.gallery_cache_capacity_mib" alias:"mcp_gallery_cache_capacity_mib" default:"256" cli:"true"`

	// DownloadPath 保存作品下载根目录；未配置时使用相对当前工作目录的路径。
	DownloadPath string `config:"download.path" alias:"download_path" env:"DOWNLOAD_PATH" default:"./downloads" cli:"true" example:"true"`

	// FilenameTemplate 保存下载文件名模板，进入首次生成的精简配置。
	FilenameTemplate string `config:"download.filename_template" alias:"filename_template" env:"FILENAME_TEMPLATE" default:"{author} - {title}_{id}" cli:"true" example:"true"`

	// DirectoryTemplate 保存可选的下载子目录模板；没有默认值，因此不进入精简配置。
	DirectoryTemplate string `config:"download.directory_template" alias:"directory_template" env:"DIRECTORY_TEMPLATE" cli:"true"`

	// HTTPSProxy 保留小写环境变量优先于大写环境变量的现有顺序，且没有默认值。
	HTTPSProxy string `config:"network.https_proxy" alias:"https_proxy" env:"https_proxy,HTTPS_PROXY" cli:"true"`

	// RequestInterval 具有运行时默认值，但不写入精简初始文件。
	RequestInterval time.Duration `config:"network.request_interval" alias:"request_interval" env:"PIXIV_REQUEST_INTERVAL" default:"0s" cli:"true"`

	// LogLevel 与 LogFormat 具有默认值并进入精简配置。
	LogLevel  LogLevel  `config:"logging.level" alias:"log_level" env:"PIXIV_LOG_LEVEL" default:"info" cli:"true" example:"true"`
	LogFormat LogFormat `config:"logging.format" alias:"log_format" env:"PIXIV_LOG_FORMAT" default:"text" cli:"true" example:"true"`

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

// OptionalString 区分「高级 TOML 键不存在」与「显式配置为空字符串」。服务级代理
// 依赖这一点：服务值为空表示直连，会覆盖全局 fallback 代理。
type OptionalString struct {
	Present bool   `json:"present"`
	Value   string `json:"value,omitempty"`
}

// ServiceNetworkConfig 保存服务级网络设置；这些键没有别名，只有用户在高级 TOML 表
// 中显式写入时才生效，不会出现在生成的初始配置里。
type ServiceNetworkConfig struct {
	ProxyURL  OptionalString `config:"proxy_url" json:"proxy_url"`
	UserAgent OptionalString `config:"user_agent" json:"user_agent"`
}

// PixivNetworkConfig 只接受 proxy_url：Pixiv 没有服务级 user_agent，独立类型让
// `pixiv.network.user_agent` 无法表达，也无法被绑定。
type PixivNetworkConfig struct {
	ProxyURL OptionalString `config:"proxy_url" json:"proxy_url"`
}

// FlareSolverrConfig 保存可选的 Cloudflare challenge 恢复服务地址。整张表缺失时该字段
// 为 nil，默认运行时不依赖 solver。
type FlareSolverrConfig struct {
	URL      string `config:"url" json:"url"`
	ProxyURL string `config:"proxy_url" json:"proxy_url,omitempty"`
}

// LogLevel 枚举日志级别配置的取值。
type LogLevel string

// LogLevelInfo、LogLevelDebug、LogLevelWarn 与 LogLevelError 是 [RuntimeConfig.LogLevel]
// 接受的全部值，也是命令行诊断输出的级别。
const (
	LogLevelInfo  LogLevel = "info"
	LogLevelDebug LogLevel = "debug"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogFormat 枚举诊断输出格式配置的取值。
type LogFormat string

// LogFormatText 输出带来源位置的文本，LogFormatJSON 输出 JSON 对象。
const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// AccountPoolStrategy 选择账号池在多个可用账号之间的调度方式。
type AccountPoolStrategy string

// AccountPoolStrategyRoundRobin 按 sort_order 轮转，AccountPoolStrategyRandom 随机选取。
const (
	AccountPoolStrategyRoundRobin AccountPoolStrategy = "round_robin"
	AccountPoolStrategyRandom     AccountPoolStrategy = "random"
)

// AccountPoolConfig 描述内容读取和作品下载是否启用数据库账号调度及其策略。
//
// UID、冻结时间和 marker 不进入 config.toml，策略枚举的有效性由领域读取负责。
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
