package settings

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

import (
	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/v2"
)

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
// LoadSnapshotAtWithFileStore 从明确路径与注入的文件端口生成一次不可变快照。
//
// 配置绑定整体分四个阶段，本函数只负责**阶段 1**：
//
//  1. 固定本次读取的文件与环境视图，避免同一个快照内出现不同结果；
//  2. 按**存在性**选择来源（见 Effective）；
//  3. 按既有输入契约解析类型并执行领域校验（见 coerceSettingValue 与
//     Runtime 中的领域规则）；
//  4. 返回类型化配置；读取过程**不回写**用户文件（写回只属 document.go）。
//
// 阶段 1 的具体含义：文件内容在此一次性装入 koanf，环境变量在此一次性捕获；
// 之后无论 Effective 被调用多少次，同一快照都看到同一份输入。
func LoadSnapshotAtWithFileStore(path string, store FileStore) (Snapshot, error) {
	store, err := requireFileStore(store)
	if err != nil {
		return Snapshot{}, err
	}
	// 阶段 1：装入文件视图。
	fileState := koanf.New(".")
	if err := loadConfigFileInto(fileState, path, store.ReadFile); err != nil {
		return Snapshot{}, err
	}
	// 阶段 1：捕获环境视图。两者共同构成这个不可变快照的输入。
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

// Effective 解析单个配置别名的最终值，返回该值及其来源。
//
// 契约：
//   - 来源优先级固定为 env → file → default；返回的 Source 就是实际选中的那一层。
//   - 选择依据是**存在性**，不是"值是否非零"：显式配置的空字符串、false、0s
//     都与"缺失"区分开，并且一旦某层命中就不再回退到更低优先级。
//   - 全部未命中时返回 Source="unset"、HasValue=false，调用方据此区分
//     "没有配置"与"配置成了零值"。
//   - 退役键（spec.Removed）仅在**显式出现**于文件时报 removed_setting；缺失时静默
//     返回 unset，因此 config unset 仍能清理它。
//
// 这是绑定四阶段中的**阶段 2**（按存在性选择来源）；类型解析与领域校验由
// coerceSettingValue（阶段 3）负责，本函数不回写任何文件（阶段 4）。
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
	// 阶段 2：env 优先。存在即为命中（即使值为空），不回退到 file。
	if raw, ok := s.env[spec.Alias]; ok && raw.present {
		return coerceSettingValue(spec, raw.value, "env")
	}
	// 阶段 2：其次文件。同样按存在性判断，保留显式空值与显式 false。
	if s.file.Exists(spec.KoanfKey) {
		return coerceSettingValue(spec, s.file.Get(spec.KoanfKey), "file")
	}
	// 阶段 2：最后默认值（来自字段的 default 标签）。
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
	pixivNetwork, err := s.pixivNetwork()
	if err != nil {
		return RuntimeConfig{}, err
	}
	fanboxNetwork, err := s.serviceNetwork("fanbox.network")
	if err != nil {
		return RuntimeConfig{}, err
	}
	reverseSearchNetwork, err := s.serviceNetwork("reverse_search.network")
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

// pixivNetwork 绑定 Pixiv 服务级网络。它只读取 proxy_url：Pixiv 不接受服务级
// user_agent，因此即使配置文件里写了 `pixiv.network.user_agent` 也不得生效
// （该差异由 PixivNetworkConfig 这个窄类型在类型层面保证）。
func (s Snapshot) pixivNetwork() (PixivNetworkConfig, error) {
	proxyURL, err := s.optionalString("pixiv.network.proxy_url")
	if err != nil {
		return PixivNetworkConfig{}, err
	}
	return PixivNetworkConfig{ProxyURL: proxyURL}, nil
}

// serviceNetwork 绑定 FANBOX / 反搜的服务级网络。它们同时支持 proxy_url 与
// user_agent，两者都保持"缺失 vs 显式空串"的可区分语义。
func (s Snapshot) serviceNetwork(prefix string) (ServiceNetworkConfig, error) {
	proxyURL, err := s.optionalString(prefix + ".proxy_url")
	if err != nil {
		return ServiceNetworkConfig{}, err
	}
	userAgent, err := s.optionalString(prefix + ".user_agent")
	if err != nil {
		return ServiceNetworkConfig{}, err
	}
	return ServiceNetworkConfig{ProxyURL: proxyURL, UserAgent: userAgent}, nil
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
