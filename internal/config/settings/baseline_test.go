package settings_test

import (
	"bytes"
	"strings"
	"testing"

	config "github.com/FlanChanXwO/pixiv-cli/internal/config/settings"
	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件是 goal-1 阶段 B 的 t8 行为层：证明初始 config.toml 的内容**完全由声明驱动**
// ——进入文件的键集合与写入值分别由 example 与 default 标签决定，而不是由第二份
// 手写的"示例清单"维护。
//
// 与 t4 的分工：t4 断言"标签本身"（反射读取）与生成文件的一致性；本文件从**文档
// 结构**反向验证，直接遍历生成的 TOML 树，因此即使标签与生成逻辑同时改错、彼此
// 仍然自洽，也会被这里发现。t4 的 TestExampleTagControlsTheGeneratedBaselineFile
// 保持不变。

// generatedConfigKeys 解析生成的初始文件，返回 "table.key" -> 文本值。
// 使用真实 TOML 解析而不是字符串匹配，避免子串误判（例如 request_interval 与
// 其他含 "interval" 的键）。
func generatedConfigKeys(t *testing.T, body []byte) map[string]string {
	t.Helper()
	doc, err := tomledit.Parse(bytes.NewReader(body))
	require.NoError(t, err, "generated baseline must be valid TOML")

	keys := make(map[string]string)
	collect := func(prefix []string, items []parser.Item) {
		for _, item := range items {
			kv, ok := item.(*parser.KeyValue)
			if !ok {
				continue
			}
			name := append(append([]string(nil), prefix...), kv.Name...)
			keys[strings.Join(name, ".")] = kv.Value.String()
		}
	}
	// 顶层（无 section）的键。
	if doc.Global != nil {
		collect(nil, doc.Global.Items)
	}
	for _, section := range doc.Sections {
		var prefix []string
		if section.Heading != nil {
			prefix = []string(section.Heading.Name)
		}
		collect(prefix, section.Items)
	}
	return keys
}

// TestBaselineFileKeysAreExactlyTheExampleTaggedDefaults 断言生成文件的键集合恰好
// 等于"声明了 example:\"true\" 且具有 default"的别名集合——不多不少。
// 这是 example 与 default 解耦的可观察证据：
//   - 有 default 但未开 example 的键（request_interval）必须缺席；
//   - 开了 example 但没有 default 的键无法写入（schema 已拒绝，这里作为不变量复核）。
func TestBaselineFileKeysAreExactlyTheExampleTaggedDefaults(t *testing.T) {
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
	got := generatedConfigKeys(t, files.files["injected/config.toml"])

	// 从公开派生元数据计算"应当进入文件"的键集合。
	want := make(map[string]string)
	for _, alias := range config.ValidSettingAliases() {
		spec, ok := config.SettingSpecByAlias(alias)
		require.Truef(t, ok, "alias %q must resolve", alias)
		if !spec.DefaultInFile {
			continue
		}
		require.Truef(t, spec.HasDefault, "alias %q declares DefaultInFile without a default", alias)
		want[spec.KoanfKey] = ""
	}

	require.Equalf(t, len(want), len(got),
		"generated key count must equal the example-tagged set\nwant keys: %v\ngot keys: %v",
		sortedKeys(want), sortedKeys(got))
	for key := range want {
		assert.Containsf(t, got, key, "example-tagged key %q must appear in the generated baseline", key)
	}
	for key := range got {
		assert.Containsf(t, want, key, "generated key %q is not declared example:\"true\"", key)
	}
}

// TestBaselineValuesComeFromTheDefaultDeclaration 断言写进文件的值逐字来自
// default 标签解释出的 Go 值，而不是另一份手工维护的示例值。
func TestBaselineValuesComeFromTheDefaultDeclaration(t *testing.T) {
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
	got := generatedConfigKeys(t, files.files["injected/config.toml"])

	// 每个键的文本必须能通过公开解析器还原为 spec.Default，且往返稳定。
	for key, text := range got {
		alias := aliasForKoanfKey(t, key)
		spec, ok := config.SettingSpecByAlias(alias)
		require.Truef(t, ok, "key %q must map to a resolvable alias", key)
		require.Truef(t, spec.HasDefault, "key %q is written without a declared default", key)

		raw := strings.Trim(text, `"`)
		value, _, err := config.ParseSettingInput(alias, raw)
		require.NoErrorf(t, err, "generated value %q for %q must parse", text, alias)
		assert.Equalf(t, spec.Default, value.Value,
			"alias %q: baseline value %q does not match its default declaration", alias, text)
	}

	// 只要 default 标签改变，文件内容就必须改变（证明没有第二份示例表）。
	// 用 request_interval 的反面证明：它未开 example，因此它的默认值不在文件里。
	assert.NotContains(t, got, "network.request_interval",
		"a key with a default but no example:\"true\" must stay out of the baseline")
}

// TestBaselineNeverWritesSecretOrRemovedKeys 断言敏感键与退役墓碑永远不进入初始文件。
func TestBaselineNeverWritesSecretOrRemovedKeys(t *testing.T) {
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
	body := string(files.files["injected/config.toml"])
	got := generatedConfigKeys(t, files.files["injected/config.toml"])

	for _, alias := range config.ValidSettingAliases() {
		spec, ok := config.SettingSpecByAlias(alias)
		require.True(t, ok)
		if !spec.Sensitive {
			continue
		}
		assert.Falsef(t, spec.DefaultInFile, "sensitive alias %q must not be example-tagged", alias)
		assert.NotContainsf(t, got, spec.KoanfKey, "sensitive key %q must never enter the baseline", spec.KoanfKey)
		assert.NotContainsf(t, body, spec.Key, "sensitive key %q must never enter the baseline", spec.Key)
	}

	// 退役键（不再出现在 ValidSettingAliases）也不得写入。
	for _, tombstone := range []string{"web.fallback_enabled", "account_pool.accounts"} {
		assert.NotContainsf(t, got, tombstone, "removed key %q must never enter the baseline", tombstone)
	}
}

// TestBaselineSectionsAreDeterministicAndGrouped 断言 section 分组来自 config 标签的
// table 前缀，且生成结果可重复（同一声明两次生成必须逐字节相同）。
func TestBaselineSectionsAreDeterministicAndGrouped(t *testing.T) {
	generate := func() []byte {
		files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
		require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
		return files.files["injected/config.toml"]
	}

	first := generate()
	second := generate()
	assert.Equal(t, string(first), string(second), "generation must be deterministic")

	got := generatedConfigKeys(t, first)
	for key := range got {
		parts := strings.Split(key, ".")
		require.GreaterOrEqualf(t, len(parts), 2, "key %q must live in a table", key)
		alias := aliasForKoanfKey(t, key)
		spec, ok := config.SettingSpecByAlias(alias)
		require.True(t, ok)
		assert.Equalf(t, spec.Table, parts[:len(parts)-1],
			"alias %q must be grouped under its declared table", alias)
	}
}

// TestEnsureDefaultConfigFileNeverOverwritesAndUsesPrivateSemantics 断言
// "已存在不覆盖"与私密文件写入语义在标签驱动之后仍成立。
func TestEnsureDefaultConfigFileNeverOverwritesAndUsesPrivateSemantics(t *testing.T) {
	path := "injected/config.toml"
	files := &injectedFileStore{path: path, files: make(map[string][]byte)}

	// 首次：生成并写一次。
	require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
	require.Equal(t, 1, files.ensured, "first Ensure must write")
	generated := append([]byte(nil), files.files[path]...)

	// 第二次：已有文件，不得覆盖，也不得再次写入。
	require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
	require.Equal(t, 2, files.ensured, "Ensure is called again but must not overwrite")
	assert.Equal(t, string(generated), string(files.files[path]), "existing file must be preserved byte-for-byte")

	// 用户内容（含注释与自定义 section）必须原样保留。
	user := []byte("# mine\n[download]\npath = \"./custom\"\n\n[custom]\nkey = 1\n")
	files.files[path] = append([]byte(nil), user...)
	require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
	assert.Equal(t, string(user), string(files.files[path]), "user document must be preserved")
}

// aliasForKoanfKey 返回给定 TOML 路径对应的别名，用于把生成文件反查回声明。
func aliasForKoanfKey(t *testing.T, key string) string {
	t.Helper()
	for _, alias := range config.ValidSettingAliases() {
		spec, ok := config.SettingSpecByAlias(alias)
		require.True(t, ok)
		if spec.KoanfKey == key {
			return alias
		}
	}
	t.Fatalf("generated key %q has no declared alias", key)
	return ""
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}
