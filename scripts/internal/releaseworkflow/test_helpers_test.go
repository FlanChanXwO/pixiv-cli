package releaseworkflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	workflowyaml "github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml"
	"gopkg.in/yaml.v3"
)

func releaseWorkflowRoot(t *testing.T) *yaml.Node {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse release workflow: %v", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		t.Fatal("release workflow must have exactly one document")
	}
	return document.Content[0]
}

func mustMarshalYAML(t *testing.T, node *yaml.Node) []byte {
	t.Helper()
	body, err := yaml.Marshal(node)
	if err != nil {
		t.Fatalf("marshal workflow fixture: %v", err)
	}
	return body
}

func jobNode(t *testing.T, root *yaml.Node, name string) *yaml.Node {
	t.Helper()
	return requireMappingValue(t, requireMappingValue(t, root, "jobs"), name)
}

func requireMappingValue(t *testing.T, mapping *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value, ok := workflowyaml.MappingValue(mapping, key)
	if !ok {
		t.Fatalf("mapping has no %q key", key)
	}
	return value
}

func findFirstUses(t *testing.T, node *yaml.Node) *yaml.Node {
	t.Helper()
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			if node.Content[index].Value == "uses" {
				return node.Content[index+1]
			}
			if found := findFirstUses(t, node.Content[index+1]); found != nil {
				return found
			}
		}
	}
	for _, child := range node.Content {
		if found := findFirstUses(t, child); found != nil {
			return found
		}
	}
	return nil
}

func removeMappingKeyRecursively(node *yaml.Node, key string) {
	if node.Kind == yaml.MappingNode {
		kept := make([]*yaml.Node, 0, len(node.Content))
		for index := 0; index+1 < len(node.Content); index += 2 {
			if node.Content[index].Value == key {
				continue
			}
			removeMappingKeyRecursively(node.Content[index+1], key)
			kept = append(kept, node.Content[index], node.Content[index+1])
		}
		node.Content = kept
		return
	}
	for _, child := range node.Content {
		removeMappingKeyRecursively(child, key)
	}
}

func stepWithRun(t *testing.T, job *yaml.Node, command string) *yaml.Node {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	for _, step := range steps.Content {
		run, ok := workflowyaml.MappingValue(step, "run")
		if ok && strings.Contains(run.Value, command) {
			return step
		}
	}
	t.Fatalf("job has no step running %q", command)
	return nil
}

func checkoutStep(t *testing.T, job *yaml.Node) *yaml.Node {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	for _, step := range steps.Content {
		uses, ok := workflowyaml.MappingValue(step, "uses")
		if ok && strings.HasPrefix(uses.Value, "actions/checkout@") {
			return step
		}
	}
	t.Fatal("job has no checkout step")
	return nil
}

func removeCommand(t *testing.T, step *yaml.Node, command string) {
	t.Helper()
	run := requireMappingValue(t, step, "run")
	lines := strings.Split(run.Value, "\n")
	kept := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if strings.TrimSpace(line) == command {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	if !removed {
		t.Fatalf("step did not run %q", command)
	}
	run.Value = strings.Join(kept, "\n")
}

func removeRunFragment(t *testing.T, step *yaml.Node, fragment string) {
	t.Helper()
	run := requireMappingValue(t, step, "run")
	if !strings.Contains(run.Value, fragment) {
		t.Fatalf("step did not contain %q", fragment)
	}
	run.Value = strings.Replace(run.Value, fragment, "", 1)
}

func replaceRunFragment(t *testing.T, step *yaml.Node, old, new string) {
	t.Helper()
	run := requireMappingValue(t, step, "run")
	if !strings.Contains(run.Value, old) {
		t.Fatalf("step did not contain %q", old)
	}
	run.Value = strings.Replace(run.Value, old, new, 1)
}

func appendRunStep(t *testing.T, job *yaml.Node, name, run string) {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	steps.Content = append(steps.Content, runStepNode(name, run))
}

func insertRunStep(t *testing.T, job *yaml.Node, index int, name, run string) {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	if index < 0 || index > len(steps.Content) {
		t.Fatalf("step index %d is outside the workflow", index)
	}
	steps.Content = append(steps.Content, nil)
	copy(steps.Content[index+1:], steps.Content[index:])
	steps.Content[index] = runStepNode(name, run)
}

func runStepNode(name, run string) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		scalarNode("name"), scalarNode(name),
		scalarNode("shell"), scalarNode("bash"),
		scalarNode("run"), scalarNode(run),
	}}
}

func appendMappingValue(t *testing.T, mapping *yaml.Node, key string, value *yaml.Node) {
	t.Helper()
	if mapping.Kind != yaml.MappingNode {
		t.Fatal("append mapping value to a non-mapping node")
	}
	mapping.Content = append(mapping.Content, scalarNode(key), value)
}

func removeMappingValue(t *testing.T, mapping *yaml.Node, key string) {
	t.Helper()
	if mapping.Kind != yaml.MappingNode {
		t.Fatal("remove mapping value from a non-mapping node")
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value != key {
			continue
		}
		mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
		return
	}
	t.Fatalf("mapping has no %q key", key)
}

func appendBracketSecretToFirstStep(t *testing.T, job *yaml.Node, expression string) {
	t.Helper()
	steps := requireMappingValue(t, job, "steps")
	appendMappingValue(t, steps.Content[0], "env", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		scalarNode("UNEXPECTED_SECRET"), scalarNode(expression),
	}})
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

func mappingNode(entries ...any) *yaml.Node {
	if len(entries)%2 != 0 {
		panic("mappingNode requires key-value pairs")
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for index := 0; index < len(entries); index += 2 {
		key, ok := entries[index].(string)
		if !ok {
			panic("mappingNode keys must be strings")
		}
		value, ok := entries[index+1].(*yaml.Node)
		if !ok {
			panic("mappingNode values must be YAML nodes")
		}
		node.Content = append(node.Content, scalarNode(key), value)
	}
	return node
}

func requiredQualityGateCommands() []string {
	return []string{
		"go run ./scripts/releaseassets validate-source --version \"${RELEASE_TAG#v}\" --product-skill skills/pixiv-cli/SKILL.md",
		"sh scripts/test-rust-vendor.sh",
		"cargo fmt --check",
		"cargo clippy --locked --offline --all-targets -- -D warnings",
		"go test ./...",
		"go test -race ./...",
		"go vet ./...",
		"go run ./scripts/licensebundle --check",
		"sh scripts/test-package-release.sh",
		"python -m pip install --disable-pip-version-check pre-commit==4.6.0",
		"python -m pre_commit run --all-files",
	}
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current directory: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil && info.Mode().IsRegular() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not find repository root")
		}
		directory = parent
	}
}
