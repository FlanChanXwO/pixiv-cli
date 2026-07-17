package workflowpolicy_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

func TestRejectAmbiguousYAMLRejectsNilNode(t *testing.T) {
	t.Parallel()

	err := workflowpolicy.RejectAmbiguousYAML(nil)
	if err == nil || err.Error() != "workflow must not contain nil YAML nodes" {
		t.Fatalf("RejectAmbiguousYAML(nil) error = %v, want exact nil-node error", err)
	}
}

func TestRejectAmbiguousYAMLRejectsAliasNode(t *testing.T) {
	t.Parallel()

	err := workflowpolicy.RejectAmbiguousYAML(&yaml.Node{Kind: yaml.AliasNode})
	if err == nil || err.Error() != "workflow must not use YAML aliases" {
		t.Fatalf("RejectAmbiguousYAML(alias) error = %v, want exact alias error", err)
	}
}

func TestRejectAmbiguousYAMLRejectsOddMapping(t *testing.T) {
	t.Parallel()

	node := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Value: "orphan"}}}
	err := workflowpolicy.RejectAmbiguousYAML(node)
	if err == nil || err.Error() != "workflow mappings must contain key-value pairs" {
		t.Fatalf("RejectAmbiguousYAML(odd mapping) error = %v, want exact key-value-pairs error", err)
	}
}

func TestRejectAmbiguousYAMLRejectsNonScalarMappingKey(t *testing.T) {
	t.Parallel()

	node := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.SequenceNode},
		{Kind: yaml.ScalarNode, Value: "value"},
	}}
	err := workflowpolicy.RejectAmbiguousYAML(node)
	if err == nil || err.Error() != "workflow mapping keys must be scalars" {
		t.Fatalf("RejectAmbiguousYAML(non-scalar key) error = %v, want exact scalar-key error", err)
	}
}

func TestRejectAmbiguousYAMLRejectsMergeKey(t *testing.T) {
	t.Parallel()

	node := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "<<"},
		{Kind: yaml.MappingNode},
	}}
	err := workflowpolicy.RejectAmbiguousYAML(node)
	if err == nil || err.Error() != "workflow must not use YAML merge keys" {
		t.Fatalf("RejectAmbiguousYAML(merge key) error = %v, want exact merge-key error", err)
	}
}

func TestRejectAmbiguousYAMLRejectsDuplicateMappingKey(t *testing.T) {
	t.Parallel()

	node := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "jobs"},
		{Kind: yaml.MappingNode},
		{Kind: yaml.ScalarNode, Value: "jobs"},
		{Kind: yaml.MappingNode},
	}}
	err := workflowpolicy.RejectAmbiguousYAML(node)
	if err == nil || err.Error() != `workflow must not contain duplicate mapping key "jobs"` {
		t.Fatalf("RejectAmbiguousYAML(duplicate key) error = %v, want exact duplicate-key error", err)
	}
}

func TestRejectAmbiguousYAMLRecursivelyRejectsAmbiguity(t *testing.T) {
	t.Parallel()

	node := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{
		{Kind: yaml.SequenceNode, Content: []*yaml.Node{{Kind: yaml.AliasNode}}},
	}}
	err := workflowpolicy.RejectAmbiguousYAML(node)
	if err == nil || err.Error() != "workflow must not use YAML aliases" {
		t.Fatalf("RejectAmbiguousYAML(nested alias) error = %v, want exact alias error", err)
	}
}

func TestMappingValueReturnsRequestedValue(t *testing.T) {
	t.Parallel()

	want := &yaml.Node{Kind: yaml.ScalarNode, Value: "release"}
	mapping := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "name"},
		want,
	}}
	got, ok := workflowpolicy.MappingValue(mapping, "name")
	if !ok || got != want {
		t.Fatalf("MappingValue(mapping, name) = (%v, %v), want exact value node", got, ok)
	}
}

func TestMappingValueReportsAbsentForMissingOrNonMappingInput(t *testing.T) {
	t.Parallel()

	mapping := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "name"},
		{Kind: yaml.ScalarNode, Value: "release"},
	}}
	for _, node := range []*yaml.Node{mapping, nil, {Kind: yaml.SequenceNode}} {
		if value, ok := workflowpolicy.MappingValue(node, "missing"); ok || value != nil {
			t.Fatalf("MappingValue(%v, missing) = (%v, %v), want (nil, false)", node, value, ok)
		}
	}
}

func TestHasExactMappingKeysRequiresTheWholeAllowlist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mapping *yaml.Node
		keys    []string
		want    bool
	}{
		{
			name: "exact keys in either order",
			mapping: &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "shell"}, {Kind: yaml.ScalarNode, Value: "bash"},
				{Kind: yaml.ScalarNode, Value: "run"}, {Kind: yaml.ScalarNode, Value: "go test ./..."},
			}},
			keys: []string{"run", "shell"},
			want: true,
		},
		{
			name: "missing key",
			mapping: &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "run"}, {Kind: yaml.ScalarNode, Value: "go test ./..."},
			}},
			keys: []string{"run", "shell"},
		},
		{
			name: "unlisted key",
			mapping: &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "run"}, {Kind: yaml.ScalarNode, Value: "go test ./..."},
				{Kind: yaml.ScalarNode, Value: "env"}, {Kind: yaml.MappingNode},
			}},
			keys: []string{"run", "shell"},
		},
		{
			name: "duplicate allowed key",
			mapping: &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "run"}, {Kind: yaml.ScalarNode, Value: "first"},
				{Kind: yaml.ScalarNode, Value: "run"}, {Kind: yaml.ScalarNode, Value: "second"},
			}},
			keys: []string{"run", "shell"},
		},
		{name: "nil mapping", keys: []string{"run"}},
		{name: "non-mapping", mapping: &yaml.Node{Kind: yaml.SequenceNode}, keys: []string{"run"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := workflowpolicy.HasExactMappingKeys(test.mapping, test.keys...); got != test.want {
				t.Fatalf("HasExactMappingKeys() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRequireScalarPreservesExactErrors(t *testing.T) {
	t.Parallel()

	mapping := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "shell"},
		{Kind: yaml.ScalarNode, Value: "bash"},
		{Kind: yaml.ScalarNode, Value: "env"},
		{Kind: yaml.MappingNode},
	}}
	if err := workflowpolicy.RequireScalar(mapping, "shell", "bash"); err != nil {
		t.Fatalf("RequireScalar(valid) error = %v", err)
	}
	for _, test := range []struct {
		name string
		key  string
		want string
	}{
		{name: "wrong value", key: "shell", want: "pwsh"},
		{name: "missing value", key: "run", want: "go test ./..."},
		{name: "non-scalar value", key: "env", want: "none"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := workflowpolicy.RequireScalar(mapping, test.key, test.want)
			wantError := test.key + ` must equal "` + test.want + `"`
			if err == nil || err.Error() != wantError {
				t.Fatalf("RequireScalar(%q, %q) error = %v, want %q", test.key, test.want, err, wantError)
			}
		})
	}
}

func TestPinnedRustToolchainReturnsAuditedReleaseProvenance(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"x86_64-apple-darwin":       "1.96.0",
		"aarch64-apple-darwin":      "1.96.1",
		"x86_64-unknown-linux-gnu":  "1.96.1",
		"aarch64-unknown-linux-gnu": "1.96.1",
		"x86_64-pc-windows-msvc":    "1.96.0",
		"aarch64-pc-windows-msvc":   "1.96.1",
	}
	for target, toolchain := range want {
		got, ok := workflowpolicy.PinnedRustToolchain(target)
		if !ok || got != toolchain {
			t.Errorf("PinnedRustToolchain(%q) = (%q, %v), want (%q, true)", target, got, ok, toolchain)
		}
	}
	if got, ok := workflowpolicy.PinnedRustToolchain("unsupported-target"); ok || got != "" {
		t.Fatalf("PinnedRustToolchain(unsupported) = (%q, %v), want (empty, false)", got, ok)
	}
}

func TestContainsSecretReferenceFindsExpressionRecursively(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node *yaml.Node
		want bool
	}{
		{
			name: "nested mixed-case context",
			node: &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
				{Kind: yaml.MappingNode, Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "run"},
					{Kind: yaml.ScalarNode, Value: "${{ toJSON(SeCrEtS) }}"},
				}},
			}},
			want: true,
		},
		{
			name: "bracket reference",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: `${{ secrets['RELEASE_SIGNING_PRIVATE_KEY'] }}`},
			want: true,
		},
		{
			name: "formatted dot reference",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: `${{ format('{0}', secrets.KEY) }}`},
			want: true,
		},
		{
			name: "formatted bracket reference after brace literal",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: `${{ format('{{{0}}}', secrets['KEY']) }}`},
			want: true,
		},
		{
			name: "formatted serialized context",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: `${{ format('{0}', toJSON(secrets)) }}`},
			want: true,
		},
		{
			name: "multiline expression",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: `${{
format(
  '{0}',
  secrets.KEY
)
}}`},
			want: true,
		},
		{
			name: "single quoted secret literal",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: `${{ format('{0}', 'secrets.KEY') }}`},
		},
		{
			name: "plain secret text between expressions",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: `${{ github.ref }} secrets.KEY ${{ github.sha }}`},
		},
		{
			name: "closing braces inside string before secret",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: `${{ format('}}', secrets.KEY) }}`},
			want: true,
		},
		{
			name: "escaped quote and brace before secret",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: `${{ format('it''s }', secrets.KEY) }}`},
			want: true,
		},
		{
			name: "secret text inside identifiers",
			node: &yaml.Node{Kind: yaml.ScalarNode, Value: `${{ notsecrets.KEY }} ${{ secrets2.KEY }}`},
		},
		{name: "plain text outside expression", node: &yaml.Node{Kind: yaml.ScalarNode, Value: "do not print secrets"}},
		{name: "singular context", node: &yaml.Node{Kind: yaml.ScalarNode, Value: "${{ secret.KEY }}"}},
		{name: "nil node"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := workflowpolicy.ContainsSecretReference(test.node); got != test.want {
				t.Fatalf("ContainsSecretReference() = %v, want %v", got, test.want)
			}
		})
	}
}
