// Same-package tests for declaration compilation: synthetic types exercise the
// errors production tags cannot produce. Public behaviour lives in the
// owner-matched settings_test files.

package settings

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSchemaRejectsInvalidDeclarations(t *testing.T) {
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

	type leaf struct {
		Value string `config:"value"`
	}
	type recursive struct {
		Next *recursive `config:"next"`
	}
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
		{"integer default overflow", struct {
			Value int64 `config:"test.value" alias:"test_value" default:"9223372036854775808"`
		}{}, "is not an integer"},
		{"fractional integer default", struct {
			Value int64 `config:"test.value" alias:"test_value" default:"1.5"`
		}{}, "is not an integer"},
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
		{"cyclic group", recursive{}, "cyclic"},
		{"unsupported slice", struct {
			Values []string `config:"test.values"`
		}{}, "unsupported"},
		{"unsupported optional type", struct {
			Value *OptionalString `config:"test.value"`
		}{}, "unsupported"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := deriveSchemaFromTags(reflect.TypeOf(test.declaration))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want error containing %q, got %v", test.want, err)
			}
		})
	}

	for _, attribute := range []string{`cli:"true"`, `example:"true"`, `env:"PRIVATE_CONFIG_TEST"`} {
		t.Run("private leaf "+attribute, func(t *testing.T) {
			declaration := reflect.StructOf([]reflect.StructField{{
				Name: "Value", Type: reflect.TypeOf(""),
				Tag: reflect.StructTag(`config:"private.value" default:"" ` + attribute),
			}})
			if _, err := deriveSchemaFromTags(declaration); err == nil {
				t.Errorf("private leaf with %s must not expose public configuration", attribute)
			}
		})
	}
}

// TestSchemaAcceptsDeclaredShapes covers the accepted side of the compiler:
// explicit false/0s/empty defaults stay distinct, groups resolve relative to
// their prefix, excluded subtrees disappear, and optional strings stay leaves.
func TestSchemaAcceptsDeclaredShapes(t *testing.T) {
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

	type hidden struct {
		Value string `config:"hidden.value" alias:"hidden_value"`
	}
	type network struct {
		Proxy string `config:"proxy_url"`
	}
	type grouped struct {
		Hidden  hidden   `config:"-"`
		Fanbox  network  `config:"fanbox.network"`
		Reverse *network `config:"reverse_search.network"`
	}
	entries, err = deriveSchemaFromTags(reflect.TypeOf(grouped{}))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, entry := range entries {
		paths = append(paths, entry.spec.TOMLPath)
		if entry.spec.Alias != "" || entry.spec.CLIManaged || entry.spec.DefaultInFile {
			t.Errorf("private leaf leaked public metadata: %#v", entry.spec)
		}
	}
	want := []string{"fanbox.network.proxy_url", "reverse_search.network.proxy_url"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}

	optional := struct {
		Network struct {
			Proxy OptionalString `config:"proxy_url"`
		} `config:"fanbox.network"`
	}{}
	entries, err = deriveSchemaFromTags(reflect.TypeOf(optional))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].spec.TOMLPath != "fanbox.network.proxy_url" || entries[0].spec.Kind != settingString {
		t.Fatalf("optional string must remain one leaf: %#v", entries)
	}
}
