package settings

import (
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
