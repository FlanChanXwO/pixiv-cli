package settings

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// loadSnapshotAt 从明确路径与文件端口生成一次不可变快照。只有 Store 方法
// 对外提供读取入口。
//
// 配置绑定整体分四个阶段，本函数只负责**阶段 1**：
//
//  1. 固定本次读取的文件与环境视图，避免同一个快照内出现不同结果；
//  2. 按**存在性**选择来源（见 Effective）；
//  3. 按既有输入契约解析类型并执行领域校验（见 coerceSettingValue 与
//     Runtime 中的领域规则）；
//  4. 返回类型化配置；读取过程**不回写**用户文件（写回只属 document.go）。
//
// 阶段 1 的具体含义：文件内容在此一次性解析为嵌套表，环境变量在此一次性捕获；
// 之后无论 Effective 被调用多少次，同一快照都看到同一份输入。
func loadSnapshotAt(path string, store FileStore) (Snapshot, error) {
	// 阶段 1：解析文件视图。
	file, err := loadConfigFile(path, store.ReadFile)
	if err != nil {
		return Snapshot{}, err
	}
	// 阶段 1：捕获环境视图。两者共同构成这个不可变快照的输入。
	return Snapshot{file: file, env: captureEnvironment()}, nil
}

// captureEnvironment 固定一次 snapshot 的环境 precedence。若 Effective 每次重新
// 读取 os.Environ，命令运行中修改环境会让同一个 snapshot 产生不同结果。
func captureEnvironment() map[Key]string {
	values := make(map[Key]string)
	for _, entry := range mustSettingSpecs() {
		if raw, present := envValue(entry.spec); present {
			values[entry.spec.Alias] = raw
		}
	}
	return values
}

// loadConfigFile 返回 TOML 文件的嵌套表。缺失或空文件等价于空表。
func loadConfigFile(path string, readFile func(string) ([]byte, error)) (map[string]any, error) {
	body, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}
	var file map[string]any
	if err := toml.Unmarshal(body, &file); err != nil {
		return nil, err
	}
	return file, nil
}

// lookup 按 TOML 路径读取嵌套表；第二个返回值区分键不存在与显式零值。
func lookup(table map[string]any, path []string) (any, bool) {
	for index, key := range path {
		value, ok := table[key]
		if !ok {
			return nil, false
		}
		if index == len(path)-1 {
			return value, true
		}
		if table, ok = value.(map[string]any); !ok {
			return nil, false
		}
	}
	return nil, false
}

// Effective 解析单个配置别名的最终值，返回该值及其来源。
//
// 契约：
//   - 来源优先级固定为 env → file → default；返回的 Source 就是实际选中的那一层。
//   - 选择依据是**存在性**，不是"值是否非零"：显式配置的空字符串、false、0s
//     都与"缺失"区分开，并且一旦某层命中就不再回退到更低优先级。
//   - 全部未命中时返回 Source="unset"、HasValue=false，调用方据此区分
//     "没有配置"与"配置成了零值"。
//
// 这是绑定四阶段中的**阶段 2**（按存在性选择来源）；类型解析与领域校验由
// coerceSettingValue（阶段 3）负责，本函数不回写任何文件（阶段 4）。
func (s Snapshot) Effective(key Key) (SettingValue, error) {
	spec, ok := SettingSpecByAlias(key)
	if !ok {
		return SettingValue{}, fmt.Errorf("unknown config key %q", key)
	}
	return s.effectiveSpec(spec)
}

// effectiveSpec 让绑定器与公开别名读取共享同一来源选择，不再查询另一份声明。
func (s Snapshot) effectiveSpec(spec SettingSpec) (SettingValue, error) {
	// 阶段 2：env 优先。存在即为命中（即使值为空），不回退到 file。
	if raw, ok := s.env[spec.Alias]; ok {
		return coerceSettingValue(spec, raw, "env")
	}
	// 阶段 2：其次文件。同样按存在性判断，保留显式空值与显式 false。
	if raw, present := s.fileValue(spec); present {
		return coerceSettingValue(spec, raw, "file")
	}
	// 阶段 2：最后默认值（来自字段的 default 标签）。
	if spec.HasDefault {
		return coerceSettingValue(spec, spec.Default, "default")
	}
	return SettingValue{Source: "unset"}, nil
}

func (s Snapshot) Runtime() (RuntimeConfig, error) {
	var cfg RuntimeConfig
	target := reflect.ValueOf(&cfg).Elem()
	// 字段声明顺序保留历史普通配置错误优先级；账号池由领域读取处理严格类型。
	entries := make([]settingSpecFromTags, 0, len(mustSettingSpecs()))
	for _, entry := range mustSettingSpecs() {
		if target.Type().Field(entry.fieldIndex[0]).Type == reflect.TypeOf(AccountPoolConfig{}) {
			continue
		}
		entries = append(entries, entry)
	}
	if err := s.bindDeclared(target, entries); err != nil {
		return RuntimeConfig{}, err
	}
	var err error
	cfg.AccountPool, err = s.accountPool()
	if err != nil {
		return RuntimeConfig{}, err
	}
	cfg.PixivNetwork, err = s.pixivNetwork()
	if err != nil {
		return RuntimeConfig{}, err
	}
	cfg.FanboxNetwork, err = s.serviceNetwork("FanboxNetwork")
	if err != nil {
		return RuntimeConfig{}, err
	}
	cfg.ReverseSearchNetwork, err = s.serviceNetwork("ReverseSearchNetwork")
	if err != nil {
		return RuntimeConfig{}, err
	}
	cfg.FanboxFlareSolverr, err = s.flareSolverr("FanboxFlareSolverr")
	if err != nil {
		return RuntimeConfig{}, err
	}
	cfg.ReverseSearchFlareSolverr, err = s.flareSolverr("ReverseSearchFlareSolverr")
	if err != nil {
		return RuntimeConfig{}, err
	}
	return cfg, nil
}

// bindDeclared 将普通标量写入本次调用的目标；静态元数据不持有运行实例。
// 私有高级叶子和可选指针组仍由领域读取决定存在性及启用条件。
func (s Snapshot) bindDeclared(target reflect.Value, entries []settingSpecFromTags) error {
	for _, entry := range entries {
		if entry.spec.Alias == "" {
			continue
		}
		field, err := target.FieldByIndexErr(entry.fieldIndex)
		if err != nil {
			// 不因声明而分配尚未启用的可选组。
			continue
		}
		value, err := s.effectiveSpec(entry.spec)
		if err != nil {
			return err
		}
		if !value.HasValue {
			continue
		}
		raw := reflect.ValueOf(value.Value)
		field.Set(raw.Convert(entry.fieldType))
	}
	return nil
}

// pixivNetwork 绑定 Pixiv 服务级网络。它只读取 proxy_url：Pixiv 不接受服务级
// user_agent，因此即使配置文件里写了 `pixiv.network.user_agent` 也不得生效
// （该差异由 PixivNetworkConfig 这个窄类型在类型层面保证）。
func (s Snapshot) pixivNetwork() (PixivNetworkConfig, error) {
	proxyURL, err := s.optionalString(runtimeFieldSpec("PixivNetwork", "ProxyURL"))
	if err != nil {
		return PixivNetworkConfig{}, err
	}
	return PixivNetworkConfig{ProxyURL: proxyURL}, nil
}

// serviceNetwork 绑定 FANBOX / 反搜的服务级网络。它们同时支持 proxy_url 与
// user_agent，两者都保持"缺失 vs 显式空串"的可区分语义。
func (s Snapshot) serviceNetwork(group string) (ServiceNetworkConfig, error) {
	proxyURL, err := s.optionalString(runtimeFieldSpec(group, "ProxyURL"))
	if err != nil {
		return ServiceNetworkConfig{}, err
	}
	userAgent, err := s.optionalString(runtimeFieldSpec(group, "UserAgent"))
	if err != nil {
		return ServiceNetworkConfig{}, err
	}
	return ServiceNetworkConfig{ProxyURL: proxyURL, UserAgent: userAgent}, nil
}

// fileValue 返回某个声明在文件中的原始值，并区分 "缺失" 与 "显式零值"。
func (s Snapshot) fileValue(spec SettingSpec) (any, bool) {
	return lookup(s.file, append(spec.Table, spec.Key))
}

func (s Snapshot) optionalString(spec SettingSpec) (OptionalString, error) {
	raw, present := s.fileValue(spec)
	if !present {
		return OptionalString{}, nil
	}
	value, ok := raw.(string)
	if !ok {
		return OptionalString{}, fmt.Errorf("%s must be a string", spec.TOMLPath)
	}
	return OptionalString{Present: true, Value: value}, nil
}

func (s Snapshot) flareSolverr(group string) (*FlareSolverrConfig, error) {
	urlSpec := runtimeFieldSpec(group, "URL")
	urlValue, err := s.optionalString(urlSpec)
	if err != nil {
		return nil, err
	}
	proxyValue, err := s.optionalString(runtimeFieldSpec(group, "ProxyURL"))
	if err != nil {
		return nil, err
	}
	if !urlValue.Present && !proxyValue.Present {
		return nil, nil
	}
	if !urlValue.Present || strings.TrimSpace(urlValue.Value) == "" {
		return nil, fmt.Errorf("%s must be set when %s is configured", urlSpec.TOMLPath, strings.Join(urlSpec.Table, "."))
	}
	return &FlareSolverrConfig{URL: urlValue.Value, ProxyURL: proxyValue.Value}, nil
}

func (s Snapshot) accountPool() (AccountPoolConfig, error) {
	enabledSpec := runtimeFieldSpec("AccountPool", "Enabled")
	strategySpec := runtimeFieldSpec("AccountPool", "Strategy")
	// 路径与默认值来自字段声明；文件值仍走严格 bool/string 与枚举校验。
	pool := AccountPoolConfig{Enabled: enabledSpec.Default.(bool), Strategy: AccountPoolStrategy(strategySpec.Default.(string))}
	if raw, present := s.fileValue(enabledSpec); present {
		enabled, ok := raw.(bool)
		if !ok {
			return AccountPoolConfig{}, fmt.Errorf("%s must be a boolean", enabledSpec.TOMLPath)
		}
		pool.Enabled = enabled
	}
	if raw, present := s.fileValue(strategySpec); present {
		value, ok := raw.(string)
		if !ok {
			return AccountPoolConfig{}, fmt.Errorf("%s must be one of: round_robin, random", strategySpec.TOMLPath)
		}
		pool.Strategy = AccountPoolStrategy(strings.TrimSpace(value))
	}
	switch pool.Strategy {
	case AccountPoolStrategyRoundRobin, AccountPoolStrategyRandom:
	default:
		return AccountPoolConfig{}, fmt.Errorf("%s must be one of: round_robin, random", strategySpec.TOMLPath)
	}
	return pool, nil
}
