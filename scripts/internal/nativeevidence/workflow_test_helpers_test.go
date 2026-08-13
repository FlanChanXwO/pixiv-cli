package nativeevidence

import (
	"os"
	"path/filepath"
	"testing"

	workflowyaml "github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml"
	"gopkg.in/yaml.v3"
)

func checkedInWorkflowRoot(t *testing.T) *yaml.Node {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "native-evidence.yml"))
	if err != nil {
		t.Fatalf("read native evidence workflow: %v", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse native evidence workflow: %v", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		t.Fatal("native evidence workflow must contain one document")
	}
	return document.Content[0]
}

func requireMappingValue(t *testing.T, mapping *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value, ok := workflowyaml.MappingValue(mapping, key)
	if !ok {
		t.Fatalf("missing mapping value %q", key)
	}
	return value
}

func requireJob(t *testing.T, root *yaml.Node) *yaml.Node {
	t.Helper()
	return requireMappingValue(t, requireMappingValue(t, root, "jobs"), "native_evidence")
}

func requireJobSteps(t *testing.T, root *yaml.Node) *yaml.Node {
	t.Helper()
	return requireMappingValue(t, requireJob(t, root), "steps")
}

func appendMappingValue(t *testing.T, mapping *yaml.Node, key string, value *yaml.Node) {
	t.Helper()
	if mapping.Kind != yaml.MappingNode {
		t.Fatalf("append mapping value %q to non-mapping", key)
	}
	mapping.Content = append(mapping.Content, scalarNode(key), value)
}

func removeMappingValue(t *testing.T, mapping *yaml.Node, key string) {
	t.Helper()
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
			return
		}
	}
	t.Fatalf("mapping value %q not found", key)
}

func removeStepNamed(t *testing.T, steps *yaml.Node, name string) {
	t.Helper()
	for index, step := range steps.Content {
		stepName, ok := workflowyaml.MappingValue(step, "name")
		if ok && stepName.Value == name {
			steps.Content = append(steps.Content[:index], steps.Content[index+1:]...)
			return
		}
	}
	t.Fatalf("step %q not found", name)
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func sequenceNode(values ...string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		node.Content = append(node.Content, scalarNode(value))
	}
	return node
}

func mappingNode(values ...any) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for index := 0; index < len(values); index += 2 {
		key, ok := values[index].(string)
		if !ok {
			panic("mapping key must be string")
		}
		value, ok := values[index+1].(*yaml.Node)
		if !ok {
			panic("mapping value must be YAML node")
		}
		node.Content = append(node.Content, scalarNode(key), value)
	}
	return node
}
