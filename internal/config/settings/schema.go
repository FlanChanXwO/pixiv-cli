package settings

import (
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
// 反射用于静态声明解析和配置绑定，不进入内容查询、分页或下载执行路径。

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
	spec       SettingSpec
	env        []string
	fieldIndex []int
	fieldType  reflect.Type
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
func envNamesFor(key Key) []string {
	for _, entry := range mustSettingSpecs() {
		if key != "" && entry.spec.Alias == key {
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
	derived, err := appendTaggedFields(derived, configType, nil, nil)
	if err != nil {
		return nil, err
	}

	// 6. 唯一性校验：重复 config 路径与重复 alias 都是 schema 错误。
	seenPaths := make(map[string]Key, len(derived))
	seenAliases := make(map[Key]string, len(derived))
	for _, entry := range derived {
		if previous, duplicate := seenPaths[entry.spec.TOMLPath]; duplicate {
			return nil, fmt.Errorf("config path %q is declared by both %q and %q", entry.spec.TOMLPath, previous, entry.spec.Alias)
		}
		seenPaths[entry.spec.TOMLPath] = entry.spec.Alias
		if previous, duplicate := seenAliases[entry.spec.Alias]; entry.spec.Alias != "" && duplicate {
			return nil, fmt.Errorf("alias %q is declared by both %q and %q", entry.spec.Alias, previous, entry.spec.TOMLPath)
		}
		if entry.spec.Alias != "" {
			seenAliases[entry.spec.Alias] = entry.spec.TOMLPath
		}
	}

	return derived, nil
}

// appendTaggedFields 解析组前缀和叶子的相对路径；排除字段不再访问其子树。
// 可选组只展开类型声明，不分配运行实例；OptionalString 是值而不是组。
func appendTaggedFields(derived []settingSpecFromTags, structType reflect.Type, pathPrefix []string, indexPrefix []int, ancestors ...reflect.Type) ([]settingSpecFromTags, error) {
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
		parts, err := configPathParts(field, rawPath)
		if err != nil {
			return nil, err
		}
		fullPath := append(append([]string(nil), pathPrefix...), parts...)
		fieldIndex := append(append([]int(nil), indexPrefix...), index)
		nested, isGroup := configGroupType(field)
		if isGroup {
			if err := validateConfigGroup(field); err != nil {
				return nil, err
			}
			derived, err = appendTaggedFields(derived, nested, fullPath, fieldIndex, ancestors...)
			if err != nil {
				return nil, err
			}
			continue
		}
		spec, err := settingSpecFromField(field, fullPath)
		if err != nil {
			return nil, err
		}
		derived = append(derived, settingSpecFromTags{
			spec:       spec,
			env:        parseEnvTag(field.Tag.Get(tagEnv)),
			fieldIndex: fieldIndex,
			fieldType:  field.Type,
		})
	}
	return derived, nil
}

func configPathParts(field reflect.StructField, rawPath string) ([]string, error) {
	parts := strings.Split(rawPath, ".")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return nil, fmt.Errorf("config field %q: config path %q contains an empty segment", field.Name, rawPath)
		}
	}
	return parts, nil
}

// configGroupType unwraps optional pointers and reports whether the field is a
// nested configuration group; OptionalString is a value, not a group.
func configGroupType(field reflect.StructField) (reflect.Type, bool) {
	nested := field.Type
	if nested.Kind() == reflect.Pointer {
		nested = nested.Elem()
	}
	return nested, nested.Kind() == reflect.Struct && nested != reflect.TypeOf(OptionalString{})
}

func validateConfigGroup(field reflect.StructField) error {
	for _, attribute := range []string{tagAlias, tagEnv, tagDefault, tagCLI, tagSecret, tagExample} {
		if _, present := field.Tag.Lookup(attribute); present {
			return fmt.Errorf("config group %q cannot declare %s", field.Name, attribute)
		}
	}
	return nil
}

func settingSpecFromField(field reflect.StructField, fullPath []string) (SettingSpec, error) {
	alias := Key(field.Tag.Get(tagAlias))
	rawPath := strings.Join(fullPath, ".")
	table, key, err := splitConfigPath(rawPath)
	if err != nil {
		return SettingSpec{}, fmt.Errorf("config field %q: %w", field.Name, err)
	}
	kind, err := kindForField(field)
	if err != nil {
		return SettingSpec{}, fmt.Errorf("config field %q: %w", field.Name, err)
	}
	spec := SettingSpec{
		Alias:    alias,
		TOMLPath: rawPath,
		Table:    table,
		Key:      key,
		Kind:     kind,
	}
	if rawDefault, hasDefault := field.Tag.Lookup(tagDefault); hasDefault {
		value, err := parseDefaultForField(field, kind, rawDefault)
		if err != nil {
			return SettingSpec{}, fmt.Errorf("config field %q (alias %q): %w", field.Name, alias, err)
		}
		spec.HasDefault = true
		spec.Default = value
	}
	for _, attribute := range []string{tagCLI, tagSecret, tagExample} {
		if raw, present := field.Tag.Lookup(attribute); present && raw != "true" && raw != "false" {
			return SettingSpec{}, fmt.Errorf("config field %q: %s must be true or false", field.Name, attribute)
		}
	}
	spec.CLIManaged = field.Tag.Get(tagCLI) == "true"
	spec.Sensitive = field.Tag.Get(tagSecret) == "true"
	spec.DefaultInFile = field.Tag.Get(tagExample) == "true"
	if alias == "" && (spec.CLIManaged || spec.DefaultInFile || len(parseEnvTag(field.Tag.Get(tagEnv))) != 0) {
		return SettingSpec{}, fmt.Errorf("config field %q: private leaf cannot declare public cli, example or env attributes", field.Name)
	}
	if spec.Sensitive && spec.DefaultInFile {
		return SettingSpec{}, fmt.Errorf("config field %q (alias %q): secret and example cannot both be true", field.Name, alias)
	}
	if spec.DefaultInFile && !spec.HasDefault {
		return SettingSpec{}, fmt.Errorf("config field %q (alias %q): example requires a default value", field.Name, alias)
	}
	return spec, nil
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
	case reflect.Int:
		return settingInteger, nil
	case reflect.Int64:
		if field.Type == reflect.TypeOf(time.Duration(0)) {
			return settingDuration, nil
		}
		if field.Type == reflect.TypeOf(int64(0)) {
			return settingInteger, nil
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
	case settingInteger:
		value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, field.Type.Bits())
		if err != nil {
			return nil, fmt.Errorf("default %q is not an integer", raw)
		}
		return value, nil
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

// SettingSpecByAlias 返回 key 对应的 spec。元数据完全来自字段标签声明。
// Table 返回独立副本，调用方修改不会污染静态声明。
func SettingSpecByAlias(key Key) (SettingSpec, bool) {
	for _, entry := range mustSettingSpecs() {
		if key != "" && entry.spec.Alias == key {
			entry.spec.Table = slices.Clone(entry.spec.Table)
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

// IsSensitiveSetting 标记禁止通过公开配置查询回显的凭据型值。
func IsSensitiveSetting(key Key) bool {
	spec, ok := SettingSpecByAlias(key)
	return ok && spec.Sensitive
}

// PublicSettingText 返回可安全进入 CLI、SDK JSON 与日志边界的配置值。
func PublicSettingText(key Key, text string) string {
	if IsSensitiveSetting(key) && text != "" {
		return "<redacted>"
	}
	return text
}

func ValidSettingAliases() []Key {
	derived := mustSettingSpecs()
	keys := make([]Key, 0, len(derived))
	for _, entry := range derived {
		if entry.spec.Alias == "" {
			continue
		}
		keys = append(keys, entry.spec.Alias)
	}
	slices.Sort(keys)
	return keys
}

// CLISettingAliases 返回由 pixiv config 命令管理的非移除配置键。
func CLISettingAliases() []Key {
	derived := mustSettingSpecs()
	keys := make([]Key, 0, len(derived))
	for _, entry := range derived {
		if entry.spec.Alias == "" || !entry.spec.CLIManaged {
			continue
		}
		keys = append(keys, entry.spec.Alias)
	}
	slices.Sort(keys)
	return keys
}

// envValue 返回 spec 对应别名按 env 标签声明顺序解析到的环境变量值。
//
// 契约：
//   - 顺序即优先级，返回第一个**存在**的变量；某个变量存在但为空也算命中，
//     绝不继续回退到后续变量（这是服务级代理"显式直连"语义依赖的行为）。
//   - 未声明 env 标签的别名返回 ("", false)。
//
// 来源完全来自字段标签，不再维护独立的 alias switch。
func envValue(spec SettingSpec) (string, bool) {
	for _, name := range envNamesFor(spec.Alias) {
		if value, present := os.LookupEnv(name); present {
			return value, true
		}
	}
	return "", false
}

// runtimeFieldSpec 按 Go 字段归属定位已编译声明，领域读取不再重复 TOML 路径。
// 参数只来自内部调用点；缺失声明是编程错误，不读取或缓存任何运行实例。
func runtimeFieldSpec(names ...string) SettingSpec {
	return declaredFieldSpec(reflect.TypeOf(RuntimeConfig{}), mustSettingSpecs(), names...)
}

func declaredFieldSpec(typ reflect.Type, entries []settingSpecFromTags, names ...string) SettingSpec {
	var index []int
	for _, name := range names {
		if typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		field, ok := typ.FieldByName(name)
		if !ok {
			panic(fmt.Sprintf("unknown configuration field %q", name))
		}
		index = append(index, field.Index...)
		typ = field.Type
	}
	for _, entry := range entries {
		if slices.Equal(entry.fieldIndex, index) {
			return entry.spec
		}
	}
	panic(fmt.Sprintf("configuration field %v has no declaration", names))
}
