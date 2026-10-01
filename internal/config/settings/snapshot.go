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
