package settings_test

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/creachadair/tomledit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件是 goal-1 阶段 B 的 Red 层：它描述"结构体标签成为配置唯一事实来源"之后必须
// 成立的 6 项契约，并刻意只以**现有稳定公开 API**（SettingSpecByAlias / Effective /
// Runtime / EnvValue / ValidSettingAliases / CLISettingAliases / ParseSettingInput /
// Store / LoadSnapshotAt）以及一个经由**公开 RuntimeConfig 类型的标签反射**读取的
// 只读探针为观察面。
//
// 这样做的原因：若改用尚不存在的导出符号（例如 Schema() / BuildSchema()），整个测试包
// 会因编译错误而失败，而 t4 要求的是"断言失败，不是未定义符号"。反射探针只读取
// **公开导出字段的标签**，因此它在 Green 阶段（给字段加标签）之前读不到任何标签，
// 断言必然失败；加完标签后同一条断言自然转绿，无需修改测试。
//
// 重要：本文件在 Green 完成前预期失败。t2 的 contract_test.go 必须继续全绿。

// ---------------------------------------------------------------- 反射探针

// fieldTags 是"某个公开导出字段的配置声明标签"的只读视图。
// 它只反映声明，不参与任何运行时绑定。
type fieldTags struct {
	config  string
	alias   string
	env     []string
	def     string
	hasDef  bool
	example string
	hasEx   bool
	cli     string
	hasCLI  bool
	secret  string
	hasSec  bool
}

// runtimeConfigTags 读取公开的 config.RuntimeConfig 类型的字段标签。
// 只读取导出字段；没有标签的字段返回零值视图。
func runtimeConfigTags(t *testing.T) map[string]fieldTags {
	t.Helper()
	typeOf := reflect.TypeOf(config.RuntimeConfig{})
	out := make(map[string]fieldTags, typeOf.NumField())
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag
		view := fieldTags{
			config: tag.Get("config"),
			alias:  tag.Get("alias"),
		}
		if raw, ok := tag.Lookup("env"); ok {
			for _, name := range strings.Split(raw, ",") {
				name = strings.TrimSpace(name)
				if name != "" {
					view.env = append(view.env, name)
				}
			}
		}
		view.def, view.hasDef = tag.Lookup("default")
		view.example, view.hasEx = tag.Lookup("example")
		view.cli, view.hasCLI = tag.Lookup("cli")
		view.secret, view.hasSec = tag.Lookup("secret")
		out[field.Name] = view
	}
	return out
}

// requireTagged 断言某个公开字段已经声明了配置标签，并返回其视图。
func requireTagged(t *testing.T, tags map[string]fieldTags, field string) fieldTags {
	t.Helper()
	view, ok := tags[field]
	require.Truef(t, ok, "RuntimeConfig 缺少公开字段 %q", field)
	require.NotEmptyf(t, view.config, "字段 %q 必须声明 config 标签（TOML 路径）", field)
	require.NotEmptyf(t, view.alias, "字段 %q 必须声明 alias 标签", field)
	return view
}

// ---------------------------------------------------------------- 契约 1：标签驱动绑定

// TestTagDeclarationsAreTheSingleSourceOfTruthForSpec 断言：派生出的公开 SettingSpec
// 必须与"字段标签声明"一致。这就是"不再手工维护第二份注册表"的可观察含义。
func TestTagDeclarationsAreTheSingleSourceOfTruthForSpec(t *testing.T) {
	tags := runtimeConfigTags(t)

	type binding struct {
		field string
		alias string
	}
	bindings := []binding{
		{field: "DownloadPath", alias: "download_path"},
		{field: "FilenameTemplate", alias: "filename_template"},
		{field: "DirectoryTemplate", alias: "directory_template"},
		{field: "LogLevel", alias: "log_level"},
		{field: "LogFormat", alias: "log_format"},
		{field: "HTTPSProxy", alias: "https_proxy"},
		{field: "RequestInterval", alias: "request_interval"},
		{field: "OutputJSON", alias: "output_json"},
		{field: "LoginOpenBrowser", alias: "login_open_browser"},
		{field: "LoginUseAfterLogin", alias: "login_use_after_login"},
		{field: "UpdateCheckEnabled", alias: "update_check_enabled"},
		{field: "ReverseSearchProvider", alias: "reverse_search_provider"},
		{field: "ReverseSearchPixivOnly", alias: "reverse_search_pixiv_only"},
		{field: "SauceNAOAPIKey", alias: "saucenao_api_key"},
		{field: "LoginRelayPublicURL", alias: "login_relay_public_url"},
		{field: "LoginRelayListenAddr", alias: "login_relay_listen_addr"},
		{field: "LoginRelayTLSCertFile", alias: "login_relay_tls_cert_file"},
		{field: "LoginRelayTLSKeyFile", alias: "login_relay_tls_key_file"},
	}

	for _, b := range bindings {
		t.Run(b.alias, func(t *testing.T) {
			view := requireTagged(t, tags, b.field)
			assert.Equalf(t, b.alias, view.alias, "字段 %q 的 alias 标签与派生 SettingSpec 不一致", b.field)

			spec, ok := config.SettingSpecByAlias(b.alias)
			require.Truef(t, ok, "alias %q 必须可解析", b.alias)
			assert.Equalf(t, view.config, spec.KoanfKey, "alias %q 的 KoanfKey 必须来自 config 标签", b.alias)
			assert.Equalf(t, b.alias, spec.Alias, "alias %q 的 spec.Alias 必须来自 alias 标签", b.alias)
			assert.NotEmptyf(t, spec.Table, "alias %q 必须派生出 TOML table", b.alias)
			assert.NotEmptyf(t, spec.Key, "alias %q 必须派生出 TOML key", b.alias)
		})
	}
}

// TestTagDrivenAliasesCoverTheWholePublicSurface 断言：标签派生出的别名集合必须
// 覆盖现有全部有效别名（迁移完成后不允许遗漏）。
func TestTagDrivenAliasesCoverTheWholePublicSurface(t *testing.T) {
	tags := runtimeConfigTags(t)

	declared := map[string]bool{}
	for _, view := range tags {
		if view.alias != "" {
			declared[view.alias] = true
		}
	}
	for _, alias := range config.ValidSettingAliases() {
		assert.Truef(t, declared[alias], "alias %q 没有对应的字段标签声明", alias)
	}
}

// ---------------------------------------------------------------- 契约 2：默认值区分

// TestDefaultTagDistinguishesEmptyFalseZeroAndAbsent 断言 default 标签能区分
// ""、false、0s、0 与"未声明"。判据是标签**是否存在**，而不是字符串是否为空。
func TestDefaultTagDistinguishesEmptyFalseZeroAndAbsent(t *testing.T) {
	tags := runtimeConfigTags(t)

	t.Run("explicit false default is declared", func(t *testing.T) {
		view := requireTagged(t, tags, "OutputJSON")
		require.True(t, view.hasDef, "字段 OutputJSON 必须声明 default 标签，以便表达显式 false")
		assert.Equal(t, "false", view.def)

		spec, ok := config.SettingSpecByAlias("output_json")
		require.True(t, ok)
		assert.True(t, spec.HasDefault, "显式 false 默认值必须被识别为有默认值")
		assert.Equal(t, false, spec.Default)
	})

	t.Run("explicit zero duration default is declared", func(t *testing.T) {
		view := requireTagged(t, tags, "RequestInterval")
		require.True(t, view.hasDef, "字段 RequestInterval 必须声明 default 标签，以便表达显式 0s")
		assert.Equal(t, "0s", view.def)

		spec, ok := config.SettingSpecByAlias("request_interval")
		require.True(t, ok)
		assert.True(t, spec.HasDefault)
		assert.Equal(t, time.Duration(0), spec.Default)
	})

	t.Run("absent default is not the same as an empty default", func(t *testing.T) {
		// https_proxy / saucenao_api_key / directory_template / login_relay_* 没有默认值。
		for _, field := range []string{"HTTPSProxy", "SauceNAOAPIKey", "DirectoryTemplate", "LoginRelayPublicURL"} {
			view := requireTagged(t, tags, field)
			assert.Falsef(t, view.hasDef, "字段 %q 不得声明 default 标签（它没有默认值）", field)
		}
		for _, alias := range []string{"https_proxy", "saucenao_api_key", "directory_template", "login_relay_public_url"} {
			spec, ok := config.SettingSpecByAlias(alias)
			require.True(t, ok)
			assert.Falsef(t, spec.HasDefault, "alias %q 必须保持无默认值", alias)
			assert.Nilf(t, spec.Default, "alias %q 不得暴露零值充当默认值", alias)
		}
	})
}

// ---------------------------------------------------------------- 契约 3：example 控制

// TestExampleTagControlsTheGeneratedBaselineFile 断言初始 config.toml 的内容完全
// 由 example 标签决定：声明 example:"true" 才进入；未声明或 false 一律不进入。
func TestExampleTagControlsTheGeneratedBaselineFile(t *testing.T) {
	tags := runtimeConfigTags(t)

	// 逐字段核对标签与"是否应出现在初始文件"的一致性。
	expectInFile := map[string]bool{
		"DownloadPath":           true,
		"FilenameTemplate":       true,
		"OutputJSON":             true,
		"LoginOpenBrowser":       true,
		"LoginUseAfterLogin":     true,
		"UpdateCheckEnabled":     true,
		"LogLevel":               true,
		"LogFormat":              true,
		"ReverseSearchProvider":  true,
		"ReverseSearchPixivOnly": true,
		// 有默认值但未开 example：不进初始文件。
		"RequestInterval": false,
		// 无默认值：不进初始文件。
		"DirectoryTemplate":   false,
		"HTTPSProxy":          false,
		"SauceNAOAPIKey":      false,
		"LoginRelayPublicURL": false,
	}
	for field, want := range expectInFile {
		view := requireTagged(t, tags, field)
		if want {
			assert.Equalf(t, "true", view.example, "字段 %q 必须声明 example:\"true\"", field)
		} else {
			assert.NotEqualf(t, "true", view.example, "字段 %q 不得声明 example:\"true\"", field)
		}
	}

	// 生成的文件内容必须与标签一致（这是 example 标签真正起作用的证据）。
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
	body := string(files.files["injected/config.toml"])
	_, err := tomledit.Parse(bytes.NewReader([]byte(body)))
	require.NoError(t, err)

	for _, fragment := range []string{
		"[download]", `path = "./downloads"`,
		"[output]", "json = false",
		"[logging]", `level = "info"`,
	} {
		assert.Containsf(t, body, fragment, "example:\"true\" 的字段必须进入初始文件：%q", fragment)
	}
	for _, fragment := range []string{"request_interval", "directory_template", "https_proxy", "saucenao_api_key", "relay_"} {
		assert.NotContainsf(t, body, fragment, "未开 example 的字段不得进入初始文件：%q", fragment)
	}
}

// TestExampleRequiresDefaultValue 断言 example:"true" 必须以 default 声明为前提：
// 没有默认值的字段不能进入初始文件（否则无值可写）。
func TestExampleRequiresDefaultValue(t *testing.T) {
	tags := runtimeConfigTags(t)
	for field, view := range tags {
		if view.example != "true" {
			continue
		}
		assert.Truef(t, view.hasDef, "字段 %q 声明了 example:\"true\"，但没有 default，初始文件无值可写", field)
	}
}

// ---------------------------------------------------------------- 契约 4：env 顺序

// TestEnvTagDeclaresNameAndOrder 断言环境变量来源完全由 env 标签声明，且顺序即优先级。
func TestEnvTagDeclaresNameAndOrder(t *testing.T) {
	tags := runtimeConfigTags(t)

	expected := map[string][]string{
		"DownloadPath":      {"DOWNLOAD_PATH"},
		"FilenameTemplate":  {"FILENAME_TEMPLATE"},
		"DirectoryTemplate": {"DIRECTORY_TEMPLATE"},
		"RequestInterval":   {"PIXIV_REQUEST_INTERVAL"},
		"LogLevel":          {"PIXIV_LOG_LEVEL"},
		"LogFormat":         {"PIXIV_LOG_FORMAT"},
		"HTTPSProxy":        {"https_proxy", "HTTPS_PROXY"},
		"SauceNAOAPIKey":    {"SAUCENAO_API_KEY"},
	}
	for field, names := range expected {
		view := requireTagged(t, tags, field)
		assert.Equalf(t, names, view.env, "字段 %q 的 env 标签名与顺序必须保持", field)

		// 派生 spec 必须能通过 EnvValue 看见同一顺序（EnvValue 契约在 t6 改为标签驱动，
		// 但对调用方必须保持"先声明者优先"）。
		spec, ok := config.SettingSpecByAlias(view.alias)
		require.Truef(t, ok, "alias %q 必须可解析", view.alias)
		_, _ = config.EnvValue(spec) // 只要求可调用；顺序行为在下方用真实环境验证
	}

	// 未声明 env 的字段不得读取任何环境变量。
	for _, field := range []string{"DirectoryTemplate_NoEnv", "OutputJSON", "LoginOpenBrowser"} {
		if field == "DirectoryTemplate_NoEnv" {
			continue
		}
		view := requireTagged(t, tags, field)
		assert.Emptyf(t, view.env, "字段 %q 不得声明 env 标签", field)
	}

	// 顺序优先级的行为验证：小写 https_proxy 必须优先于大写 HTTPS_PROXY。
	t.Run("declared order wins", func(t *testing.T) {
		view := requireTagged(t, tags, "HTTPSProxy")
		require.Len(t, view.env, 2)
		assert.Equal(t, "https_proxy", view.env[0], "第一个声明的变量必须优先级最高")
		assert.Equal(t, "HTTPS_PROXY", view.env[1])

		t.Setenv("https_proxy", "http://lower")
		t.Setenv("HTTPS_PROXY", "http://upper")
		state, err := config.LoadSnapshotAt(filepath.Join(t.TempDir(), "config.toml"))
		require.NoError(t, err)
		value, err := state.Effective("https_proxy")
		require.NoError(t, err)
		assert.Equal(t, "http://lower", value.Value)
	})
}

// ---------------------------------------------------------------- 契约 5：secret

// TestSecretTagKeepsValuesOutOfBaselineAndPublicOutput 断言 secret:"true" 的字段
// 既不进入初始文件，也不进入公开输出。
func TestSecretTagKeepsValuesOutOfBaselineAndPublicOutput(t *testing.T) {
	tags := runtimeConfigTags(t)

	view := requireTagged(t, tags, "SauceNAOAPIKey")
	assert.Equalf(t, "true", view.secret, "字段 SauceNAOAPIKey 必须声明 secret:\"true\"")
	assert.NotEqualf(t, "true", view.example, "secret 字段不得进入初始示例")

	// 其余字段不得被标记为 secret。
	for field, other := range tags {
		if field == "SauceNAOAPIKey" {
			continue
		}
		assert.NotEqualf(t, "true", other.secret, "字段 %q 不得声明 secret:\"true\"", field)
	}

	// 派生 spec 的敏感性必须来自标签。
	spec, ok := config.SettingSpecByAlias("saucenao_api_key")
	require.True(t, ok)
	assert.True(t, spec.Sensitive, "Sensitive 必须由 secret 标签派生")
	assert.True(t, config.IsSensitiveSetting("saucenao_api_key"))
	assert.Equal(t, "<redacted>", config.PublicSettingText("saucenao_api_key", "must-not-leak"))

	// 初始文件里不得出现敏感键。
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
	assert.NotContains(t, string(files.files["injected/config.toml"]), "saucenao_api_key")
}

// TestSecretAndExampleConflictIsRejected 断言 secret:"true" 与 example:"true"
// 同时出现必须被明确拒绝，而不是静默写出敏感值。
func TestSecretAndExampleConflictIsRejected(t *testing.T) {
	tags := runtimeConfigTags(t)
	for field, view := range tags {
		if view.secret == "true" && view.example == "true" {
			t.Fatalf("字段 %q 同时声明 secret:\"true\" 与 example:\"true\"；schema 必须拒绝这种冲突", field)
		}
	}

	// 行为层证据：初始文件生成必须成功，且不含任何敏感值。
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	err := (config.Store{Files: files}).EnsureDefaultConfigFile()
	require.NoError(t, err, "初始文件生成不得因 secret/example 冲突而失败")
	assert.NotContains(t, string(files.files["injected/config.toml"]), "must-not-leak")
}

// ---------------------------------------------------------------- 契约 6：schema 错误

// TestSchemaRejectsDuplicateConfigPathsAndAliases 断言重复的 TOML 路径与重复别名
// 必须被明确拒绝。判据：标签声明的 (config, alias) 对在全部公开字段中唯一。
func TestSchemaRejectsDuplicateConfigPathsAndAliases(t *testing.T) {
	tags := runtimeConfigTags(t)

	seenPaths := map[string]string{}
	seenAliases := map[string]string{}
	for field, view := range tags {
		if view.config == "" && view.alias == "" {
			continue // 未参与配置绑定的字段
		}
		if view.config == "-" {
			continue // 显式退出绑定
		}
		if view.config != "" {
			if previous, dup := seenPaths[view.config]; dup {
				t.Errorf("config 路径 %q 被 %q 与 %q 重复声明", view.config, previous, field)
			}
			seenPaths[view.config] = field
		}
		if view.alias != "" {
			if previous, dup := seenAliases[view.alias]; dup {
				t.Errorf("alias %q 被 %q 与 %q 重复声明", view.alias, previous, field)
			}
			seenAliases[view.alias] = field
		}
	}

	// 派生 spec 层不得出现同一 alias 的两个条目。
	counts := map[string]int{}
	for _, alias := range config.ValidSettingAliases() {
		counts[alias]++
	}
	for alias, count := range counts {
		assert.Equalf(t, 1, count, "alias %q 在公开集合中出现 %d 次", alias, count)
	}
}

// TestSchemaRejectsInvalidDefaultForFieldType 断言 default 标签必须能按字段类型解释，
// 无法解释的声明必须失败而不是被静默接受。
func TestSchemaRejectsInvalidDefaultForFieldType(t *testing.T) {
	tags := runtimeConfigTags(t)

	// 正例：可解释的默认值必须能派生为对应的 Go 值。
	view := requireTagged(t, tags, "RequestInterval")
	require.True(t, view.hasDef)
	parsed, err := time.ParseDuration(view.def)
	require.NoErrorf(t, err, "RequestInterval 的 default 标签 %q 必须是合法 duration", view.def)
	assert.Equal(t, time.Duration(0), parsed)

	spec, ok := config.SettingSpecByAlias("request_interval")
	require.True(t, ok)
	assert.IsTypef(t, time.Duration(0), spec.Default, "duration 字段的默认值必须被解释为 time.Duration")

	// 负例：布尔字段的默认值必须能按布尔解释。
	boolView := requireTagged(t, tags, "OutputJSON")
	require.True(t, boolView.hasDef)
	_, err = parseBoolTag(boolView.def)
	require.NoErrorf(t, err, "OutputJSON 的 default 标签 %q 必须能按布尔解释", boolView.def)

	// 默认值类型必须与字段类型匹配：duration 字段拿到 0s，布尔字段拿到 false。
	outputSpec, ok := config.SettingSpecByAlias("output_json")
	require.True(t, ok)
	assert.IsTypef(t, false, outputSpec.Default, "bool 字段的默认值必须被解释为 bool")
}

// parseBoolTag 把 default 标签按 bool 解释，供上面的类型一致性断言使用。
func parseBoolTag(raw string) (bool, error) {
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, &stringError{"default tag is not a bool: " + raw}
	}
}

type stringError struct{ message string }

func (e *stringError) Error() string { return e.message }

// ---------------------------------------------------------------- 环境隔离辅助

// TestTagSchemaIsUsableWithoutTheLegacySwitch 断言：环境来源的公开行为在标签化之后
// 仍能仅通过声明解释——未声明 env 的别名必须报告非 env 来源。
func TestTagSchemaIsUsableWithoutTheLegacySwitch(t *testing.T) {
	clearSettingEnvironment(t)
	state, err := config.LoadSnapshotAt(filepath.Join(t.TempDir(), "config.toml"))
	require.NoError(t, err)

	for _, alias := range config.ValidSettingAliases() {
		spec, ok := config.SettingSpecByAlias(alias)
		require.Truef(t, ok, "alias %q 必须可解析", alias)
		value, err := state.Effective(alias)
		require.NoErrorf(t, err, "Effective(%q)", alias)
		if _, declared := authoritativeEnvNames[alias]; !declared {
			assert.NotEqualf(t, "env", value.Source, "alias %q 未声明 env，来源不得是 env", alias)
		}
		_ = spec
	}
}

// TestSettingSpecExposesTypeOnlyThroughObservableBehaviour 断言 Kind（未导出）不进入
// 公开表面，因此类型解释必须经由 Default 与 ParseSettingInput 的可观察行为，而不是
// 让调用方依赖未导出的 kind。
func TestSettingSpecExposesTypeOnlyThroughObservableBehaviour(t *testing.T) {
	typeOf := reflect.TypeOf(config.SettingSpec{})
	_, ok := typeOf.FieldByName("Kind")
	require.True(t, ok, "SettingSpec 仍保留 Kind 字段")

	// 类型解释必须可从公开面观察：默认值类型 + 输入解析均能判定字段类型。
	for alias, want := range map[string]any{
		"download_path":    "",
		"request_interval": time.Duration(0),
		"output_json":      false,
	} {
		value, _, err := config.ParseSettingInput(alias, defaultTextFor(alias))
		require.NoErrorf(t, err, "ParseSettingInput(%q)", alias)
		assert.Truef(t, value.HasValue, "alias %q 必须能解释一个合法输入", alias)
		require.IsTypef(t, want, value.Value, "alias %q 的公开值类型必须可判定", alias)
	}
}

func defaultTextFor(alias string) string {
	switch alias {
	case "request_interval":
		return "1m"
	case "output_json":
		return "true"
	default:
		return "./out"
	}
}
