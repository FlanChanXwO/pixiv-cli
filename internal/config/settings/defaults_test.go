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

// generatedConfigKeys parses the generated baseline into "table.key" -> text.
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
	if doc.Global != nil {
		collect(nil, doc.Global.Items)
	}
	for _, section := range doc.Sections {
		var prefix []string
		if section.Heading != nil {
			prefix = section.Name
		}
		collect(prefix, section.Items)
	}
	return keys
}

// generatedBaseline writes the compact default config through the file port.
func generatedBaseline(t *testing.T) (*injectedFileStore, []byte) {
	t.Helper()
	files := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
	require.Equal(t, 1, files.ensured, "first Ensure must write")
	body, ok := files.files["injected/config.toml"]
	require.True(t, ok)
	return files, body
}

// TestGeneratedBaselineFileSurfaceIsStable covers the compact key set, values,
// section grouping and order, and the "existing file is preserved" rule.
func TestGeneratedBaselineFileSurfaceIsStable(t *testing.T) {
	files, body := generatedBaseline(t)
	got := generatedConfigKeys(t, body)

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

	// Values round-trip through the public parser back to the declared default.
	for key, text := range got {
		alias := aliasForKoanfKey(t, key)
		spec, ok := config.SettingSpecByAlias(alias)
		require.True(t, ok)
		raw := strings.Trim(text, `"`)
		value, _, err := config.ParseSettingInput(alias, raw)
		require.NoErrorf(t, err, "generated value %q for %q must parse", text, alias)
		assert.Equal(t, spec.Default, value.Value, "baseline value does not match its default declaration")
	}

	// Sections come from the declared table and are stable across runs.
	second := &injectedFileStore{path: "injected/config.toml", files: make(map[string][]byte)}
	require.NoError(t, (config.Store{Files: second}).EnsureDefaultConfigFile())
	assert.Equal(t, string(body), string(second.files["injected/config.toml"]), "generation must be deterministic")

	for key := range got {
		parts := strings.Split(key, ".")
		require.GreaterOrEqualf(t, len(parts), 2, "key %q must live in a table", key)
		alias := aliasForKoanfKey(t, key)
		spec, ok := config.SettingSpecByAlias(alias)
		require.True(t, ok)
		assert.Equal(t, spec.Table, parts[:len(parts)-1], "alias %q must be grouped under its declared table", alias)
	}

	var order []string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "[") {
			order = append(order, strings.Trim(line, "[]"))
		}
	}
	assert.Equal(t, []string{"download", "output", "login", "update", "logging", "reverse_search"}, order,
		"baseline section order is user-visible; keep it stable")

	// Sensitive aliases and removed keys never enter the generated file.
	for _, alias := range config.ValidSettingAliases() {
		spec, ok := config.SettingSpecByAlias(alias)
		require.True(t, ok)
		if !spec.Sensitive {
			continue
		}
		assert.Falsef(t, spec.DefaultInFile, "sensitive alias %q must not be example-tagged", alias)
		assert.NotContainsf(t, got, spec.KoanfKey, "sensitive key %q must never enter the baseline", spec.KoanfKey)
		assert.NotContainsf(t, string(body), spec.Key, "sensitive key %q must never enter the baseline", spec.Key)
	}
	for _, tombstone := range []string{"web.fallback_enabled", "account_pool.accounts"} {
		assert.NotContainsf(t, got, tombstone, "removed key %q must never enter the baseline", tombstone)
	}

	existing := append([]byte(nil), files.files["injected/config.toml"]...)
	files.files["injected/config.toml"] = []byte("# user content\n")
	require.NoError(t, (config.Store{Files: files}).EnsureDefaultConfigFile())
	assert.Equal(t, 2, files.ensured, "Ensure is called again but must not overwrite")
	assert.Equal(t, "# user content\n", string(files.files["injected/config.toml"]), "existing config file must never be overwritten")
	files.files["injected/config.toml"] = existing
}

// aliasForKoanfKey maps a generated TOML path back to its alias.
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
