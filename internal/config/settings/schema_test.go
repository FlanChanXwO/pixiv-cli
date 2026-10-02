package settings

import (
	"github.com/knadh/koanf/v2"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSchemaRejectsInvalidBooleanAttributes(t *testing.T) {
	for _, attribute := range []string{"cli", "secret", "example"} {
		for _, value := range []string{"", "TRUE", "1", "flase"} {
			t.Run(attribute+"="+value, func(t *testing.T) {
				declaration := reflect.StructOf([]reflect.StructField{{
					Name: "Value", Type: reflect.TypeOf(""),
					Tag: reflect.StructTag(`config:"test.value" alias:"test_value" default:"" ` + attribute + `:"` + value + `"`),
				}})
				_, err := deriveSchemaFromTags(declaration)
				if err == nil || !strings.Contains(err.Error(), attribute) {
					t.Fatalf("invalid %s attribute must be rejected, got %v", attribute, err)
				}
			})
		}
	}
}

func TestSchemaRejectsInvalidDeclarations(t *testing.T) {
	cases := []struct {
		name        string
		declaration any
		want        string
	}{
		{"secret example", struct {
			Value string `config:"test.value" alias:"test_value" secret:"true" example:"true" default:"synthetic"`
		}{}, "secret and example cannot both be true"},
		{"example without default", struct {
			Value string `config:"test.value" alias:"test_value" example:"true"`
		}{}, "example requires a default value"},
		{"duplicate path", struct {
			First  string `config:"test.value" alias:"first"`
			Second string `config:"test.value" alias:"second"`
		}{}, `config path "test.value" is declared by both`},
		{"duplicate alias", struct {
			First  string `config:"test.first" alias:"value"`
			Second string `config:"test.second" alias:"value"`
		}{}, `alias "value" is declared by both`},
		{"invalid bool default", struct {
			Value bool `config:"test.value" alias:"test_value" default:"invalid"`
		}{}, "is not a boolean"},
		{"invalid duration default", struct {
			Value time.Duration `config:"test.value" alias:"test_value" default:"invalid"`
		}{}, "is not a duration"},
		{"tombstone alias", struct {
			Value bool `config:"test.value" alias:"web_fallback_enabled"`
		}{}, `alias "web_fallback_enabled" is declared by both`},
		{"tombstone path", struct {
			Value bool `config:"account_pool.accounts" alias:"test_value"`
		}{}, `config path "account_pool.accounts" is declared by both`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := deriveSchemaFromTags(reflect.TypeOf(test.declaration))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestSchemaPreservesExplicitDefaultsAndAttributes(t *testing.T) {
	type declaration struct {
		Empty   string        `config:"test.empty" alias:"empty" default:"" cli:"false" secret:"false" example:"true"`
		False   bool          `config:"test.false" alias:"false" default:"false" cli:"true" example:"false"`
		Zero    time.Duration `config:"test.zero" alias:"zero" default:"0s" secret:"true"`
		Missing string        `config:"test.missing" alias:"missing"`
	}
	entries, err := deriveSchemaFromTags(reflect.TypeOf(declaration{}))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []any{"", false, time.Duration(0)} {
		if !entries[i].spec.HasDefault || entries[i].spec.Default != want {
			t.Errorf("field %d: want explicit default %#v, got %#v", i, want, entries[i].spec)
		}
	}
	if entries[3].spec.HasDefault {
		t.Fatal("missing default must remain absent")
	}
	if entries[0].spec.CLIManaged || entries[0].spec.Sensitive || !entries[0].spec.DefaultInFile ||
		!entries[1].spec.CLIManaged || entries[1].spec.DefaultInFile || !entries[2].spec.Sensitive {
		t.Fatal("explicit boolean attributes changed")
	}
}

func TestSchemaExcludesEntireSubtree(t *testing.T) {
	type hidden struct {
		Value string `config:"hidden.value" alias:"hidden_value"`
	}
	type declaration struct {
		Hidden hidden `config:"-"`
	}
	entries, err := deriveSchemaFromTags(reflect.TypeOf(declaration{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.spec.Removed {
			t.Errorf("excluded subtree produced configuration entry %q", entry.spec.KoanfKey)
		}
	}
}

func TestSchemaResolvesReusableGroupsRelativeToEachPrefix(t *testing.T) {
	type network struct {
		Proxy string `config:"proxy_url"`
	}
	type declaration struct {
		Fanbox  network  `config:"fanbox.network"`
		Reverse *network `config:"reverse_search.network"`
	}
	entries, err := deriveSchemaFromTags(reflect.TypeOf(declaration{}))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, entry := range entries {
		if !entry.spec.Removed {
			paths = append(paths, entry.spec.KoanfKey)
			if entry.spec.Alias != "" || entry.spec.CLIManaged || entry.spec.DefaultInFile {
				t.Errorf("private leaf leaked public metadata: %#v", entry.spec)
			}
		}
	}
	want := []string{"fanbox.network.proxy_url", "reverse_search.network.proxy_url"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestSchemaRejectsMalformedGroupsAndPaths(t *testing.T) {
	type leaf struct {
		Value string `config:"value"`
	}
	cases := []struct {
		name        string
		declaration any
		want        string
	}{
		{"empty path", struct {
			Value string `config:"" alias:"value"`
		}{}, "empty segment"},
		{"empty group segment", struct {
			Group leaf `config:"a..b"`
		}{}, "empty segment"},
		{"group alias", struct {
			Group leaf `config:"a" alias:"group"`
		}{}, "group"},
		{"group default", struct {
			Group leaf `config:"a" default:""`
		}{}, "group"},
		{"duplicate expanded path", struct {
			Group leaf   `config:"a"`
			Value string `config:"a.value" alias:"other"`
		}{}, "declared by both"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := deriveSchemaFromTags(reflect.TypeOf(test.declaration))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want %q, got %v", test.want, err)
			}
		})
	}
}

func TestSchemaKeepsOptionalStringAsPrivateLeaf(t *testing.T) {
	type network struct {
		Proxy OptionalString `config:"proxy_url"`
	}
	type declaration struct {
		Network network `config:"fanbox.network"`
	}
	entries, err := deriveSchemaFromTags(reflect.TypeOf(declaration{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1+len(settingTombstones) || entries[0].spec.KoanfKey != "fanbox.network.proxy_url" || entries[0].spec.Kind != settingString {
		t.Fatalf("optional string must remain one leaf: %#v", entries)
	}
}

func TestSchemaRejectsPublicAttributesOnPrivateLeaves(t *testing.T) {
	for _, attribute := range []string{`cli:"true"`, `example:"true"`, `env:"PRIVATE_CONFIG_TEST"`} {
		declaration := reflect.StructOf([]reflect.StructField{{
			Name: "Value", Type: reflect.TypeOf(""),
			Tag: reflect.StructTag(`config:"private.value" default:"" ` + attribute),
		}})
		if _, err := deriveSchemaFromTags(declaration); err == nil {
			t.Errorf("private leaf with %s must not expose public configuration", attribute)
		}
	}
}

func TestSchemaRejectsCyclicGroupsAndUnsupportedTypes(t *testing.T) {
	type recursive struct {
		Next *recursive `config:"next"`
	}
	cases := []struct {
		value any
		want  string
	}{
		{recursive{}, "cyclic"},
		{struct {
			Values []string `config:"test.values"`
		}{}, "unsupported"},
		{struct {
			Value *OptionalString `config:"test.value"`
		}{}, "unsupported"},
	}
	for _, test := range cases {
		_, err := deriveSchemaFromTags(reflect.TypeOf(test.value))
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%T: want %q, got %v", test.value, test.want, err)
		}
	}
}

func TestDeclaredFieldsBindSelectedValues(t *testing.T) {
	type name string
	type group struct {
		Name    name          `config:"name" alias:"test_name" default:"fallback"`
		Wait    time.Duration `config:"wait" alias:"test_wait" default:"3s"`
		Enabled bool          `config:"enabled" alias:"test_enabled" default:"true"`
		Empty   string        `config:"empty" alias:"test_empty" default:""`
		Missing string        `config:"missing" alias:"test_missing"`
	}
	type target struct {
		Value    string `config:"test.value" alias:"test_value" default:"scalar"`
		Group    group  `config:"nested"`
		Excluded group  `config:"-"`
	}
	entries, err := deriveSchemaFromTags(reflect.TypeOf(target{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, file  string
		env         map[string]snapshotEnvValue
		wantName    name
		wantWait    time.Duration
		wantEnabled bool
	}{
		{name: "defaults", wantName: "fallback", wantWait: 3 * time.Second, wantEnabled: true},
		{name: "file zero values", file: "[nested]\nname = ''\nwait = '0s'\nenabled = false", wantName: "", wantWait: 0, wantEnabled: false},
		{name: "environment empty wins", file: "[nested]\nname = 'file'", env: map[string]snapshotEnvValue{"test_name": {value: "", present: true}}, wantName: "", wantWait: 3 * time.Second, wantEnabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := koanf.New(".")
			if err := loadConfigFileInto(state, "fixture", func(string) ([]byte, error) { return []byte(tc.file), nil }); err != nil {
				t.Fatal(err)
			}
			s := Snapshot{file: state, env: tc.env}
			cfg := target{Group: group{Empty: "old", Missing: "preserved"}, Excluded: group{Name: "excluded"}}
			if err := s.bindDeclared(reflect.ValueOf(&cfg).Elem(), entries); err != nil {
				t.Fatal(err)
			}
			if cfg.Value != "scalar" || cfg.Group.Name != tc.wantName || cfg.Group.Wait != tc.wantWait || cfg.Group.Enabled != tc.wantEnabled || cfg.Group.Empty != "" || cfg.Group.Missing != "preserved" || cfg.Excluded.Name != "excluded" {
				t.Fatalf("binding result: %#v", cfg)
			}
			empty, err := s.effectiveSpec(entries[4].spec)
			if err != nil || !empty.HasValue || empty.Source != "default" || empty.Value != "" {
				t.Fatalf("explicit empty default: %#v %v", empty, err)
			}
			missing, err := s.effectiveSpec(entries[5].spec)
			if err != nil || missing.HasValue || missing.Source != "unset" {
				t.Fatalf("absent default: %#v %v", missing, err)
			}
		})
	}
}

func TestDeclaredBindingDoesNotAllocateOptionalGroupsOrCacheTargets(t *testing.T) {
	type group struct {
		Value string `config:"value" alias:"optional_value" default:"fresh"`
	}
	type target struct {
		Group *group `config:"optional"`
	}
	entries, err := deriveSchemaFromTags(reflect.TypeOf(target{}))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{file: koanf.New(".")}
	absent := target{}
	if err := snapshot.bindDeclared(reflect.ValueOf(&absent).Elem(), entries); err != nil {
		t.Fatal(err)
	}
	if absent.Group != nil {
		t.Fatal("ordinary binding allocated an optional domain group")
	}
	first, second := target{Group: &group{}}, target{Group: &group{}}
	if err := snapshot.bindDeclared(reflect.ValueOf(&first).Elem(), entries); err != nil {
		t.Fatal(err)
	}
	first.Group.Value = "changed"
	if err := snapshot.bindDeclared(reflect.ValueOf(&second).Elem(), entries); err != nil {
		t.Fatal(err)
	}
	if first.Group.Value != "changed" || second.Group.Value != "fresh" {
		t.Fatalf("targets shared binding state: %q %q", first.Group.Value, second.Group.Value)
	}
}

func TestRuntimePreservesValidationOrderAndStrictPoolTypes(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"interval before logging", "[network]\nrequest_interval='bad'\n[logging]\nlevel='bad'", `request_interval: time: invalid duration "bad"`},
		{"logging before tombstone", "[logging]\nlevel='bad'\n[web]\nfallback_enabled=true", "log_level must be one of: info, debug"},
		{"tombstone before reverse provider", "[web]\nfallback_enabled=true\n[reverse_search]\nprovider='bad'", "removed_setting: config key \"web_fallback_enabled\" was removed; clear it with `pixiv config unset web_fallback_enabled`"},
		{"pool boolean remains strict", "[account_pool]\nenabled='true'", "account_pool.enabled must be a boolean"},
		{"pool strategy remains strict", "[account_pool]\nstrategy=123", "account_pool.strategy must be one of: round_robin, random"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := koanf.New(".")
			if err := loadConfigFileInto(state, "fixture", func(string) ([]byte, error) { return []byte(tc.body), nil }); err != nil {
				t.Fatal(err)
			}
			_, err := (Snapshot{file: state}).Runtime()
			if err == nil || err.Error() != tc.want {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestAdvancedPathsArePrivateDeclarations(t *testing.T) {
	entries, err := deriveSchemaFromTags(reflect.TypeOf(RuntimeConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"pixiv.network.proxy_url", "fanbox.network.proxy_url", "fanbox.network.user_agent", "reverse_search.network.proxy_url", "reverse_search.network.user_agent", "fanbox.flaresolverr.url", "fanbox.flaresolverr.proxy_url", "reverse_search.flaresolverr.url", "reverse_search.flaresolverr.proxy_url"} {
		found := false
		for _, entry := range entries {
			if entry.spec.KoanfKey == path {
				found = true
				if entry.spec.Alias != "" || entry.spec.CLIManaged || entry.spec.DefaultInFile || len(entry.env) != 0 || len(entry.fieldIndex) != 2 {
					t.Errorf("private declaration %s: %#v", path, entry)
				}
			}
		}
		if !found {
			t.Errorf("advanced path %s has no production declaration", path)
		}
	}
}

func TestPrivateUIDDeclarationsCompileWithoutRuntimeBinding(t *testing.T) {
	for product, path := range map[string]string{"Pixiv": "pixiv.auth.default_user_id", "Fanbox": "fanbox.auth.default_user_id"} {
		spec := defaultAccountSpec(product)
		if spec.KoanfKey != path || spec.Alias != "" {
			t.Fatalf("%s production declaration: %#v", product, spec)
		}
	}

	type account struct {
		UserID int64 `config:"default_user_id"`
	}
	type selection struct {
		Pixiv  account `config:"pixiv.auth"`
		Fanbox account `config:"fanbox.auth"`
	}
	entries, err := deriveSchemaFromTags(reflect.TypeOf(selection{}))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, entry := range entries {
		if !entry.spec.Removed {
			paths = append(paths, entry.spec.KoanfKey)
			if entry.spec.Alias != "" || entry.spec.HasDefault || entry.spec.CLIManaged || entry.spec.DefaultInFile {
				t.Fatalf("UID declaration exposed publicly: %#v", entry.spec)
			}
		}
	}
	if !reflect.DeepEqual(paths, []string{"pixiv.auth.default_user_id", "fanbox.auth.default_user_id"}) {
		t.Fatalf("paths=%v", paths)
	}
	for _, entry := range mustSettingSpecs() {
		if strings.HasSuffix(entry.spec.KoanfKey, ".default_user_id") {
			t.Fatal("UID declaration entered ordinary runtime schema")
		}
	}
}

func TestUIDDeclarationsRejectPublicAliasAndDefault(t *testing.T) {
	for _, declaration := range []any{
		struct {
			UserID int64 `config:"auth.uid" alias:"uid"`
		}{},
		struct {
			UserID int64 `config:"auth.uid" default:"1"`
		}{},
	} {
		if _, err := deriveSchemaFromTags(reflect.TypeOf(declaration)); err == nil {
			t.Fatal("UID declaration must remain private and domain-parsed")
		}
	}
}

// 路径来自声明，值与严格类型预期独立；同一测试也用于只改标签的路径探针。
func TestAccountPoolUsesDeclaredPaths(t *testing.T) {
	enabled := runtimeFieldSpec("AccountPool", "Enabled")
	strategy := runtimeFieldSpec("AccountPool", "Strategy")
	for _, tc := range []struct {
		name              string
		enabled, strategy any
		wantErr           string
	}{
		{"configured", true, " random ", ""},
		{"strict boolean", "true", "random", enabled.KoanfKey + " must be a boolean"},
		{"strict strategy", true, 123, strategy.KoanfKey + " must be one of: round_robin, random"},
		{"unknown strategy", true, "weighted", strategy.KoanfKey + " must be one of: round_robin, random"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := koanf.New(".")
			if err := state.Set(enabled.KoanfKey, tc.enabled); err != nil {
				t.Fatal(err)
			}
			if err := state.Set(strategy.KoanfKey, tc.strategy); err != nil {
				t.Fatal(err)
			}
			cfg, err := (Snapshot{file: state}).Runtime()
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("want %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil || !cfg.AccountPool.Enabled || cfg.AccountPool.Strategy != AccountPoolStrategyRandom {
				t.Fatalf("declared pool values: %+v, error: %v", cfg.AccountPool, err)
			}
		})
	}
}
