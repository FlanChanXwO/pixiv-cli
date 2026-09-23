package nativeevidence

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var pinnedActionPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)

func TestNativeEvidenceWorkflowKeepsSecurityAndOwnershipBoundaries(t *testing.T) {
	t.Parallel()

	path := filepath.Join(findRepositoryRoot(t), ".github", "workflows", "native-evidence.yml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(body)
	for _, required := range []string{
		"workflow_dispatch:",
		"./tools/platformmatrix --capability native-evidence",
		"scripts/build-platform.sh",
		"--cc '${{ matrix.cc }}'",
		"CC='${{ matrix.cc }}' go test ./internal/media/ugoira",
		"scripts/cmd/nativeevidence record",
		"refs/heads/main",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("native evidence workflow missing ownership/security contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"\n  push:",
		"build-staticlibs.sh",
		"go build -trimpath",
		"releaseassets package",
		"gh release",
		"git push",
		"docker push",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("native evidence workflow owns forbidden duplicate/side-effect %q", forbidden)
		}
	}

	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse native evidence workflow: %v", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		t.Fatal("native evidence workflow must contain one mapping document")
	}
	root := document.Content[0]
	if workflowHasAmbiguousYAML(root) {
		t.Fatal("native evidence workflow must not contain aliases, merge keys, or duplicate mapping keys")
	}
	permissions := workflowMappingValue(t, root, "permissions")
	if permissions.Kind != yaml.MappingNode || len(permissions.Content) != 0 {
		t.Fatal("native evidence workflow must keep empty global permissions")
	}
	if workflowContainsSecretReference(root) {
		t.Fatal("native evidence workflow must not reference GitHub secrets")
	}

	jobs := workflowMappingValue(t, root, "jobs")
	if jobs.Kind != yaml.MappingNode {
		t.Fatal("native evidence jobs must be a mapping")
	}
	for index := 0; index+1 < len(jobs.Content); index += 2 {
		jobName := jobs.Content[index].Value
		job := jobs.Content[index+1]
		permissions := workflowMappingValue(t, job, "permissions")
		if permissions.Kind != yaml.MappingNode || len(permissions.Content) != 2 ||
			permissions.Content[0].Value != "contents" || permissions.Content[1].Value != "read" {
			t.Fatalf("native evidence job %q must have only contents: read", jobName)
		}
	}

	walkWorkflowMappings(root, func(mapping *yaml.Node) {
		if _, ok := workflowOptionalMappingValue(mapping, "environment"); ok {
			t.Error("native evidence workflow must not use a GitHub environment")
		}
		uses, ok := workflowOptionalMappingValue(mapping, "uses")
		if !ok {
			return
		}
		if uses.Kind != yaml.ScalarNode {
			t.Error("native evidence action reference must be a scalar")
			return
		}
		if !pinnedActionPattern.MatchString(uses.Value) {
			t.Errorf("GitHub action must be pinned to a full commit SHA: %s", uses.Value)
		}
		if !strings.HasPrefix(uses.Value, "actions/checkout@") {
			return
		}
		with := workflowMappingValue(t, mapping, "with")
		persist := workflowMappingValue(t, with, "persist-credentials")
		if persist.Kind != yaml.ScalarNode || persist.Value != "false" {
			t.Error("native evidence checkout must disable persisted credentials")
		}
	})
}

func workflowMappingValue(t *testing.T, mapping *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value, ok := workflowOptionalMappingValue(mapping, key)
	if !ok {
		t.Fatalf("native evidence workflow missing mapping key %q", key)
	}
	return value
}

func workflowOptionalMappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1], true
		}
	}
	return nil, false
}

func walkWorkflowMappings(node *yaml.Node, visit func(*yaml.Node)) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		visit(node)
	}
	for _, child := range node.Content {
		walkWorkflowMappings(child, visit)
	}
}

func workflowHasAmbiguousYAML(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.AliasNode {
		return true
	}
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Value == "<<" {
				return true
			}
			if _, duplicate := seen[key.Value]; duplicate {
				return true
			}
			seen[key.Value] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if workflowHasAmbiguousYAML(child) {
			return true
		}
	}
	return false
}

func workflowContainsSecretReference(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.ScalarNode {
		lowered := strings.ToLower(node.Value)
		for _, marker := range []string{"secrets.", "secrets[", "secrets ["} {
			if strings.Contains(lowered, marker) {
				return true
			}
		}
	}
	for _, child := range node.Content {
		if workflowContainsSecretReference(child) {
			return true
		}
	}
	return false
}
