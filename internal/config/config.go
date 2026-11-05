// Package config 读取本机配置、环境变量，并负责保留注释的 TOML 写回。
//
// [Config] 保存一次读取的有效值，[Default] 提供默认值；[Store] 通过文件端口读写配置。
package config

// Config 按 TOML 分节保存配置；可选指针区分缺失与显式空值。
type Config struct {
	MCP struct {
		GalleryAnimationTempCapacityMiB int64 `toml:"gallery_animation_temp_capacity_mib"`
		GalleryCacheCapacityMiB         int64 `toml:"gallery_cache_capacity_mib"`
	} `toml:"mcp"`
	Download struct {
		Path              string `toml:"path"`
		FilenameTemplate  string `toml:"filename_template"`
		DirectoryTemplate string `toml:"directory_template"`
	} `toml:"download"`
	Network struct {
		HTTPSProxy      string `toml:"https_proxy"`
		RequestInterval string `toml:"request_interval"` // Go duration 字符串，与已有文件一致。
	} `toml:"network"`
	Logging struct {
		Level  LogLevel  `toml:"level"`
		Format LogFormat `toml:"format"`
	} `toml:"logging"`
	Output struct {
		JSON bool `toml:"json"`
	} `toml:"output"`
	Update struct {
		CheckEnabled bool `toml:"check_enabled"`
	} `toml:"update"`
	Login struct {
		OpenBrowser      bool   `toml:"open_browser"`
		UseAfterLogin    bool   `toml:"use_after_login"`
		RelayPublicURL   string `toml:"relay_public_url"`
		RelayListenAddr  string `toml:"relay_listen_addr"`
		RelayTLSCertFile string `toml:"relay_tls_cert_file"`
		RelayTLSKeyFile  string `toml:"relay_tls_key_file"`
	} `toml:"login"`
	Pixiv struct {
		Network struct {
			ProxyURL *string `toml:"proxy_url"`
		} `toml:"network"`
		Auth struct {
			DefaultUserID *int64 `toml:"default_user_id"`
		} `toml:"auth"`
	} `toml:"pixiv"`
	Fanbox struct {
		Network      ServiceNetworkConfig `toml:"network"`
		FlareSolverr FlareSolverrConfig   `toml:"flaresolverr"`
		Auth         struct {
			DefaultUserID *int64 `toml:"default_user_id"`
		} `toml:"auth"`
	} `toml:"fanbox"`
	ReverseSearch struct {
		Provider       string               `toml:"provider"`
		PixivOnly      bool                 `toml:"pixiv_only"`
		SauceNAOAPIKey string               `toml:"saucenao_api_key"`
		Network        ServiceNetworkConfig `toml:"network"`
		FlareSolverr   FlareSolverrConfig   `toml:"flaresolverr"`
	} `toml:"reverse_search"`
	AccountPool AccountPoolConfig `toml:"account_pool"`
}

// ServiceNetworkConfig 的空代理值表示直连，nil 表示沿用全局代理。
type ServiceNetworkConfig struct {
	ProxyURL  *string `toml:"proxy_url"`
	UserAgent *string `toml:"user_agent"`
}

// FlareSolverrConfig 在任一字段出现时要求 URL 非空。
type FlareSolverrConfig struct {
	URL      *string `toml:"url"`
	ProxyURL *string `toml:"proxy_url"`
}

// Default 返回独立的默认配置。
func Default() Config {
	var c Config
	c.MCP.GalleryAnimationTempCapacityMiB = 1024
	c.MCP.GalleryCacheCapacityMiB = 256
	c.Download.Path = "./downloads"
	c.Download.FilenameTemplate = "{author} - {title}_{id}"
	c.Network.RequestInterval = "0s"
	c.Logging.Level = LogLevelInfo
	c.Logging.Format = LogFormatText
	c.Update.CheckEnabled = true
	c.Login.OpenBrowser = true
	c.ReverseSearch.Provider = "saucenao"
	c.ReverseSearch.PixivOnly = true
	c.AccountPool.Strategy = AccountPoolStrategyRoundRobin
	return c
}

// LogLevel 枚举日志级别配置的取值。
type LogLevel string

// LogLevelInfo、LogLevelDebug、LogLevelWarn 与 LogLevelError 是 [Config]
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
	Enabled  bool                `toml:"enabled"`
	Strategy AccountPoolStrategy `toml:"strategy"`
}
