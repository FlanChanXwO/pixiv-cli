package settings

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 本文件把配置声明从"手写的 settingSpecs 事实表"迁移为"结构体字段标签"。
// 字段标签是唯一事实来源；本文件负责把它派生为内部元数据，并继续通过既有的
// 公开 SettingSpec 视图对外提供，避免 CLI 消费者感知迁移。
//
// 反射只用于**声明期**解析：派生结果在首次使用时构建一次并被复用，不进入
// Runtime()、内容查询、分页或下载等执行热路径。

// 标签名。它们由本项目的绑定实现解释，不是 Go 或 TOML 库的原生标签。
const (
	tagConfig  = "config"  // TOML 路径；"-" 表示不参与配置绑定
	tagAlias   = "alias"   // 现有配置别名
	tagEnv     = "env"     // 允许读取的环境变量，逗号分隔且顺序即优先级
	tagDefault = "default" // 缺失时的默认值，按字段类型解释
	tagExample = "example" // 是否进入首次生成的精简配置；仅 "true" 进入
	tagCLI     = "cli"     // 是否由 config get/set/unset 管理
	tagSecret  = "secret"  // 是否必须在公开输出中隐藏且禁止进入初始示例
)

// settingSpecFromTags 是标签声明派生出的内部元数据。SettingSpec 是它的公开视图。
type settingSpecFromTags struct {
	spec SettingSpec
	env  []string
}

// schemaOnce 只缓存静态声明；声明错误由 mustSettingSpecs 作为编程错误报告。
var schemaOnce = sync.OnceValues(func() ([]settingSpecFromTags, error) {
	return deriveSchemaFromTags(reflect.TypeOf(RuntimeConfig{}))
})

// settingSpecsFromTags 返回派生元数据；schema 声明错误在此处暴露。
func settingSpecsFromTags() ([]settingSpecFromTags, error) {
	return schemaOnce()
}

// envNamesFor 返回某个别名按 env 标签声明顺序排列的环境变量名。顺序即优先级，
// 因此这里不排序也不去重：调用方按顺序返回第一个**存在**的变量即可。
func envNamesFor(alias string) []string {
	for _, entry := range mustSettingSpecs() {
		if alias != "" && entry.spec.Alias == alias {
			return entry.env
		}
	}
	return nil
}

// deriveSchemaFromTags 读取配置类型（含嵌套配置组）的公开字段标签并派生
// 配置元数据。它同时执行 schema 校验：重复 config 路径、重复 alias、非法默认值、
// example 缺少默认值，以及 secret 与 example 的冲突都会返回明确错误。
func deriveSchemaFromTags(configType reflect.Type) ([]settingSpecFromTags, error) {
	// 1. 按声明顺序遍历公开字段，跳过不参与绑定的字段，并递归展开嵌套配置组。
	derived := make([]settingSpecFromTags, 0, 24)
	derived, err := appendTaggedFields(derived, configType, nil)
	if err != nil {
		return nil, err
	}

	// 6. 唯一性校验：重复 config 路径与重复 alias 都是 schema 错误。
	seenPaths := make(map[string]string, len(derived))
	seenAliases := make(map[string]string, len(derived))
	for _, entry := range derived {
		if previous, duplicate := seenPaths[entry.spec.KoanfKey]; duplicate {
			return nil, fmt.Errorf("config path %q is declared by both %q and %q", entry.spec.KoanfKey, previous, entry.spec.Alias)
		}
		seenPaths[entry.spec.KoanfKey] = entry.spec.Alias
		if previous, duplicate := seenAliases[entry.spec.Alias]; entry.spec.Alias != "" && duplicate {
			return nil, fmt.Errorf("alias %q is declared by both %q and %q", entry.spec.Alias, previous, entry.spec.KoanfKey)
		}
		if entry.spec.Alias != "" {
			seenAliases[entry.spec.Alias] = entry.spec.KoanfKey
		}
	}

	// 7. 追加不绑定到 RuntimeConfig 字段的迁移墓碑。它们仍可被查询，以便
	//    config unset 清理旧配置，且必须与已声明别名保持唯一性。
	for _, tombstone := range settingTombstones {
		if previous, duplicate := seenAliases[tombstone.Alias]; duplicate {
			return nil, fmt.Errorf("alias %q is declared by both %q and %q", tombstone.Alias, previous, tombstone.KoanfKey)
		}
		seenAliases[tombstone.Alias] = tombstone.KoanfKey
		if previous, duplicate := seenPaths[tombstone.KoanfKey]; duplicate {
			return nil, fmt.Errorf("config path %q is declared by both %q and %q", tombstone.KoanfKey, previous, tombstone.Alias)
		}
		seenPaths[tombstone.KoanfKey] = tombstone.Alias
		derived = append(derived, settingSpecFromTags{spec: tombstone})
	}

	return derived, nil
}

// appendTaggedFields 解析组前缀和叶子的相对路径；排除字段不再访问其子树。
// 可选组只展开类型声明，不分配运行实例；OptionalString 是值而不是组。
func appendTaggedFields(derived []settingSpecFromTags, structType reflect.Type, pathPrefix []string, ancestors ...reflect.Type) ([]settingSpecFromTags, error) {
	if structType == nil || structType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("configuration declaration must be a struct")
	}
	if slices.Contains(ancestors, structType) {
		return nil, fmt.Errorf("cyclic configuration group %s", structType)
	}
	ancestors = append(ancestors, structType)
	for index := 0; index < structType.NumField(); index++ {
		field := structType.Field(index)
		if !field.IsExported() {
			continue
		}
		rawPath, hasPath := field.Tag.Lookup(tagConfig)
		if !hasPath || rawPath == "-" {
			continue
		}
		parts := strings.Split(rawPath, ".")
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				return nil, fmt.Errorf("config field %q: config path %q contains an empty segment", field.Name, rawPath)
			}
		}
		fullPath := append(append([]string(nil), pathPrefix...), parts...)
		nested := field.Type
		if nested.Kind() == reflect.Pointer {
			nested = nested.Elem()
		}
		if nested.Kind() == reflect.Struct && nested != reflect.TypeOf(OptionalString{}) {
			for _, attribute := range []string{tagAlias, tagEnv, tagDefault, tagCLI, tagSecret, tagExample} {
				if _, present := field.Tag.Lookup(attribute); present {
					return nil, fmt.Errorf("config group %q cannot declare %s", field.Name, attribute)
				}
			}
			var err error
			derived, err = appendTaggedFields(derived, nested, fullPath, ancestors...)
			if err != nil {
				return nil, err
			}
			continue
		}
		alias := field.Tag.Get(tagAlias)
		rawPath = strings.Join(fullPath, ".")
		table, key, err := splitConfigPath(rawPath)
		if err != nil {
			return nil, fmt.Errorf("config field %q: %w", field.Name, err)
		}
		kind, err := kindForField(field)
		if err != nil {
			return nil, fmt.Errorf("config field %q: %w", field.Name, err)
		}

		spec := SettingSpec{
			Alias:    alias,
			KoanfKey: rawPath,
			Table:    table,
			Key:      key,
			Kind:     kind,
		}

		// 2. 默认值：用标签**是否存在**判断，而不是"字符串是否为空"。
		//    这样 default:"" / default:"false" / default:"0s" / default:"0" 都能与
		//    "未声明 default" 明确区分。
		if rawDefault, hasDefault := field.Tag.Lookup(tagDefault); hasDefault {
			value, err := parseDefaultForField(field, kind, rawDefault)
			if err != nil {
				return nil, fmt.Errorf("config field %q (alias %q): %w", field.Name, alias, err)
			}
			spec.HasDefault = true
			spec.Default = value
		}

		// 开发者属性仅接受 true/false，避免拼写错误静默关闭安全或公开行为。
		// 这里不改变用户配置值的既有解析规则。
		for _, attribute := range []string{tagCLI, tagSecret, tagExample} {
			if raw, present := field.Tag.Lookup(attribute); present && raw != "true" && raw != "false" {
				return nil, fmt.Errorf("config field %q: %s must be true or false", field.Name, attribute)
			}
		}
		spec.CLIManaged = field.Tag.Get(tagCLI) == "true"
		spec.Sensitive = field.Tag.Get(tagSecret) == "true"
		spec.DefaultInFile = field.Tag.Get(tagExample) == "true"
		if alias == "" && (spec.CLIManaged || spec.DefaultInFile || len(parseEnvTag(field.Tag.Get(tagEnv))) != 0) {
			return nil, fmt.Errorf("config field %q: private leaf cannot declare public cli, example or env attributes", field.Name)
		}

		// 4. schema 校验：secret 与 example 冲突必须明确拒绝，而不是静默写出敏感值。
		if spec.Sensitive && spec.DefaultInFile {
			return nil, fmt.Errorf("config field %q (alias %q): secret and example cannot both be true", field.Name, alias)
		}
		// 5. example 必须以 default 为前提，否则初始文件无值可写。
		if spec.DefaultInFile && !spec.HasDefault {
			return nil, fmt.Errorf("config field %q (alias %q): example requires a default value", field.Name, alias)
		}

		derived = append(derived, settingSpecFromTags{
			spec: spec,
			env:  parseEnvTag(field.Tag.Get(tagEnv)),
		})
	}
	return derived, nil
}

// settingTombstones 是已退役但必须保持可查询的配置键。它们不绑定字段、不驱动
// 运行时分支，只用于让旧配置的显式写入得到 removed_setting 并允许 unset 清理。
var settingTombstones = []SettingSpec{
	{
		Alias:    "web_fallback_enabled",
		KoanfKey: "web.fallback_enabled",
		Table:    []string{"web"},
		Key:      "fallback_enabled",
		Kind:     settingBool,
		Removed:  true,
	},
	{
		Alias:    "account_pool_accounts",
		KoanfKey: "account_pool.accounts",
		Table:    []string{"account_pool"},
		Key:      "accounts",
		Kind:     settingString,
		Removed:  true,
	},
}

// parseEnvTag 拆分 env 标签：逗号分隔，去空白，保持声明顺序。
// 顺序即优先级，因此这里绝不排序或去重。
func parseEnvTag(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	names := make([]string, 0, 2)
	for _, name := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

// splitConfigPath 把 "download.path" 拆成 TOML table 与 key。
func splitConfigPath(path string) ([]string, string, error) {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return nil, "", fmt.Errorf("config path %q must contain a table and a key", path)
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return nil, "", fmt.Errorf("config path %q contains an empty segment", path)
		}
	}
	return append([]string(nil), parts[:len(parts)-1]...), parts[len(parts)-1], nil
}

// kindForField 识别已有标量及保留存在性的 OptionalString，不展开值类型的内部字段。
func kindForField(field reflect.StructField) (settingKind, error) {
	if field.Type == reflect.TypeOf(OptionalString{}) {
		return settingString, nil
	}
	switch field.Type.Kind() {
	case reflect.String:
		return settingString, nil
	case reflect.Bool:
		return settingBool, nil
	case reflect.Int64:
		if field.Type == reflect.TypeOf(time.Duration(0)) {
			return settingDuration, nil
		}
		return "", fmt.Errorf("unsupported integer field type %s", field.Type)
	default:
		return "", fmt.Errorf("unsupported configuration field type %s", field.Type)
	}
}

// parseDefaultForField 按字段类型解释 default 标签的值。
// 解释失败是 schema 错误，不得被静默接受。
func parseDefaultForField(field reflect.StructField, kind settingKind, raw string) (any, error) {
	switch kind {
	case settingString:
		return raw, nil
	case settingBool:
		value, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("default %q is not a boolean", raw)
		}
		return value, nil
	case settingDuration:
		value, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("default %q is not a duration", raw)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported setting kind %q", kind)
	}
}

// SettingSpecByAlias 返回 alias 对应的 spec。已移除键仍可被查询，以便 config unset
// 执行清理、config get/set 返回 removed_setting。元数据完全来自字段标签声明。
func SettingSpecByAlias(alias string) (SettingSpec, bool) {
	for _, entry := range mustSettingSpecs() {
		if alias != "" && entry.spec.Alias == alias {
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
		if entry.spec.Removed || entry.spec.Alias == "" {
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
		if entry.spec.Removed || entry.spec.Alias == "" || !entry.spec.CLIManaged {
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
