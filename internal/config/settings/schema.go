package settings

import (
	"fmt"
	"reflect"
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

// schemaOnce 保证派生结果只构建一次。schemaErr 记录构建期间发现的声明错误，
// 由调用方决定在何处暴露（当前公开访问器保持既有签名，因此错误在构建期以
// panic 之外的方式记录并在 schema 自检中可见；见 schemaProblem）。
var schemaOnce = sync.OnceValues(func() ([]settingSpecFromTags, error) {
	return deriveSchemaFromTags()
})

// settingSpecsFromTags 返回派生元数据；schema 声明错误在此处暴露。
func settingSpecsFromTags() ([]settingSpecFromTags, error) {
	return schemaOnce()
}

// deriveSchemaFromTags 读取 RuntimeConfig（含嵌套配置组）的公开字段标签并派生
// 配置元数据。它同时执行 schema 校验：重复 config 路径、重复 alias、非法默认值、
// example 缺少默认值，以及 secret 与 example 的冲突都会返回明确错误。
func deriveSchemaFromTags() ([]settingSpecFromTags, error) {
	// 1. 按声明顺序遍历公开字段，跳过不参与绑定的字段，并递归展开嵌套配置组。
	derived := make([]settingSpecFromTags, 0, 24)
	derived, err := appendTaggedFields(derived, reflect.TypeOf(RuntimeConfig{}), nil)
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
		if previous, duplicate := seenAliases[entry.spec.Alias]; duplicate {
			return nil, fmt.Errorf("alias %q is declared by both %q and %q", entry.spec.Alias, previous, entry.spec.KoanfKey)
		}
		seenAliases[entry.spec.Alias] = entry.spec.KoanfKey
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

// appendTaggedFields 递归收集一个配置结构体（含嵌套配置组）中带 config 标签的
// 公开字段。嵌套结构体自身声明 config:"-" 表示"不是独立配置项，其字段各自声明路径"。
func appendTaggedFields(derived []settingSpecFromTags, structType reflect.Type, pathPrefix []string) ([]settingSpecFromTags, error) {
	for index := 0; index < structType.NumField(); index++ {
		field := structType.Field(index)
		if !field.IsExported() {
			continue
		}
		rawPath, hasPath := field.Tag.Lookup(tagConfig)

		// 未声明 config 的字段不参与绑定。
		if !hasPath || rawPath == "" {
			continue
		}
		// 显式退出绑定：既可能是真正的非配置字段，也可能是需要递归展开的配置组。
		if rawPath == "-" {
			nested, nestedErr := nestedConfigStruct(field)
			if nestedErr != nil {
				return nil, fmt.Errorf("config field %q: %w", field.Name, nestedErr)
			}
			if nested == nil {
				continue
			}
			var expandErr error
			derived, expandErr = appendTaggedFields(derived, nested, pathPrefix)
			if expandErr != nil {
				return nil, expandErr
			}
			continue
		}

		alias, hasAlias := field.Tag.Lookup(tagAlias)
		if !hasAlias || alias == "" {
			return nil, fmt.Errorf("config field %q declares config=%q without an alias", field.Name, rawPath)
		}

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

		// 3. 布尔属性：只有显式 "true" 才算开启。
		spec.CLIManaged = field.Tag.Get(tagCLI) == "true"
		spec.Sensitive = field.Tag.Get(tagSecret) == "true"
		spec.DefaultInFile = field.Tag.Get(tagExample) == "true"

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

// nestedConfigStruct 判断一个 config:"-" 字段是否是需要递归展开的配置组。
// 只有值类型结构体（非指针、非切片、非 map）才会被展开；指针类型是可选配置组，
// 由各自的领域绑定负责，因此这里返回 nil 表示不展开。
func nestedConfigStruct(field reflect.StructField) (reflect.Type, error) {
	if field.Type.Kind() != reflect.Struct {
		return nil, nil
	}
	// 不含任何配置标签的结构体不是配置组。
	for index := 0; index < field.Type.NumField(); index++ {
		if _, ok := field.Type.Field(index).Tag.Lookup(tagConfig); ok {
			return field.Type, nil
		}
	}
	return nil, nil
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

// kindForField 把 Go 字段类型映射为配置类型。只有这三种字段类型允许参与配置绑定。
func kindForField(field reflect.StructField) (settingKind, error) {
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
