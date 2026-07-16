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
