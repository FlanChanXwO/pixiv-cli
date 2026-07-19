package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflowpolicy"
	"gopkg.in/yaml.v3"
)

// 预发布验收必须是只读的：它只允许手动触发并验证一个已公开的 stable Release。
func TestWorkflowEnforcesReadOnlyStableReleaseRehearsal(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "homebrew-prepublish-verify.yml"))
	if err != nil {
		t.Fatalf("read prepublish Homebrew workflow: %v", err)
	}
	if err := checkWorkflow(body); err != nil {
		t.Fatalf("prepublish Homebrew workflow policy rejected the checked-in workflow: %v", err)
	}
}

// Homebrew 已明确拒绝未配合 --build-from-source 的 --debug-symbols。预发布演练必须仅
// 在 Linux 使用这个正式、非交互的参数组合；macOS 保持普通安装命令，旧 --keep-tmp 不得残留。
func TestWorkflowUsesLinuxOnlySourceBuildDebugSymbolsForHomebrewResourceStaging(t *testing.T) {
	root := prepublishWorkflowRoot(t)
	step := runStepWith(t, job(t, root, "verify_homebrew_formula"), "brew install")
	run := mappingValue(t, step, "run").Value
	want := `if [ '${{ matrix.os }}' = linux ]; then
  brew install --build-from-source --debug-symbols --verbose --formula "$staging_tap/$formula_name"
else
  brew install --formula "$staging_tap/$formula_name"
fi`
	if !strings.Contains(run, want) {
		t.Fatalf("prepublish install must retain Resource staging sources only on Linux and preserve macOS install; missing %q", want)
	}
	if strings.Contains(run, "--keep-tmp") {
		t.Fatal("prepublish install must not retain obsolete --keep-tmp")
	}
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func TestWorkflowRejectsReadOnlyBoundaryMutations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root *yaml.Node)
	}{
		{
			name: "adds a push trigger",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, mappingValue(t, root, "on"), "push", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
			},
		},
		{
			name: "adds a deploy job",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, mappingValue(t, root, "jobs"), "deploy", &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
			},
		},
		{
			name: "adds an environment",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, job(t, root, "render_stable_formula"), "environment", scalarNode("release"))
			},
		},
		{
			name: "references a secret",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendMappingValue(t, job(t, root, "verify_homebrew_formula"), "env", mappingNode("LEAK", scalarNode("${{ secrets.TOKEN }}")))
			},
		},
		{
			name: "removes Linux source build required by debug symbols",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRun(t, runStepWith(t, job(t, root, "verify_homebrew_formula"), "brew install --build-from-source"), "brew install --build-from-source --debug-symbols --verbose", "brew install --debug-symbols --verbose")
			},
		},
		{
			name: "changes the required Linux debug-symbol ordering",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRun(t, runStepWith(t, job(t, root, "verify_homebrew_formula"), "brew install --build-from-source"), "brew install --build-from-source --debug-symbols --verbose", "brew install --debug-symbols --build-from-source --verbose")
			},
		},
		{
			name: "removes Linux Resource staging debug symbols",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRun(t, runStepWith(t, job(t, root, "verify_homebrew_formula"), "brew install --build-from-source"), "brew install --build-from-source --debug-symbols --verbose", "brew install --build-from-source --verbose")
			},
		},
		{
			name: "retains obsolete Linux keep tmp",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRun(t, runStepWith(t, job(t, root, "verify_homebrew_formula"), "brew install --build-from-source"), "brew install --build-from-source --debug-symbols --verbose", "brew install --build-from-source --debug-symbols --keep-tmp --verbose")
			},
		},
		{
			name: "applies Linux debug symbols to macOS",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRun(t, runStepWith(t, job(t, root, "verify_homebrew_formula"), "brew install --build-from-source"), "brew install --formula \"$staging_tap/$formula_name\"", "brew install --build-from-source --debug-symbols --verbose --formula \"$staging_tap/$formula_name\"")
			},
		},
		{
			name: "downloads an unverified file",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRun(t, runStepWith(t, job(t, root, "render_stable_formula"), "gh release download"), "--pattern checksums.txt", "--pattern checksums.json")
			},
		},
		{
			name: "uses a mutable action reference",
			mutate: func(t *testing.T, root *yaml.Node) {
				mappingValue(t, mappingValue(t, job(t, root, "validate_existing_release"), "steps").Content[0], "uses").Value = "actions/checkout@v4"
			},
		},
		{
			name: "validate creates a Release after checking it",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendRun(t, runStepWith(t, job(t, root, "validate_existing_release"), "gh api"), "gh release create \"$RELEASE_TAG\" --title unsafe")
			},
		},
		{
			name: "checksum download pushes a tap commit",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendRun(t, runStepWith(t, job(t, root, "render_stable_formula"), "gh release download"), "git push origin HEAD:main")
			},
		},
		{
			name: "checksum download trusts the public tap",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendRun(t, runStepWith(t, job(t, root, "render_stable_formula"), "gh release download"), "brew tap \"FlanChanXwO/tap\"")
			},
		},
		{
			name: "native verification creates a Release through the API",
			mutate: func(t *testing.T, root *yaml.Node) {
				appendRun(t, runStepWith(t, job(t, root, "verify_homebrew_formula"), "pixiv version --json"), "gh api --method POST \"repos/$GITHUB_REPOSITORY/releases\"")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := prepublishWorkflowRoot(t)
			test.mutate(t, root)
			if err := checkWorkflow(marshalWorkflow(t, root)); err == nil {
				t.Fatal("policy accepted an unsafe prepublish workflow mutation")
			}
		})
	}
}

func TestQualityWorkflowRunsPrepublishPolicy(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read quality workflow: %v", err)
	}
	if !strings.Contains(string(body), "sh scripts/test-homebrew-prepublish-workflow.sh") {
		t.Fatal("quality workflow does not run the prepublish Homebrew policy")
	}
}

func prepublishWorkflowRoot(t *testing.T) *yaml.Node {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(findRepositoryRoot(t), ".github", "workflows", "homebrew-prepublish-verify.yml"))
	if err != nil {
		t.Fatalf("read prepublish workflow: %v", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse prepublish workflow: %v", err)
	}
	return document.Content[0]
}

func marshalWorkflow(t *testing.T, root *yaml.Node) []byte {
	t.Helper()
	body, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal workflow: %v", err)
	}
	return body
}

func job(t *testing.T, root *yaml.Node, name string) *yaml.Node {
	t.Helper()
	return mappingValue(t, mappingValue(t, root, "jobs"), name)
}

func mappingValue(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value, ok := workflowpolicy.MappingValue(node, key)
	if !ok {
		t.Fatalf("mapping has no %q", key)
	}
	return value
}

func runStepWith(t *testing.T, job *yaml.Node, fragment string) *yaml.Node {
	t.Helper()
	for _, step := range mappingValue(t, job, "steps").Content {
		if run, ok := workflowpolicy.MappingValue(step, "run"); ok && strings.Contains(run.Value, fragment) {
			return step
		}
	}
	t.Fatalf("job has no run step containing %q", fragment)
	return nil
}

func replaceRun(t *testing.T, step *yaml.Node, old, new string) {
	t.Helper()
	run := mappingValue(t, step, "run")
	if !strings.Contains(run.Value, old) {
		t.Fatalf("run step has no %q", old)
	}
	run.Value = strings.Replace(run.Value, old, new, 1)
}

func appendRun(t *testing.T, step *yaml.Node, command string) {
	t.Helper()
	run := mappingValue(t, step, "run")
	run.Value += "\n" + command
}

func appendMappingValue(t *testing.T, node *yaml.Node, key string, value *yaml.Node) {
	t.Helper()
	node.Content = append(node.Content, scalarNode(key), value)
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func mappingNode(entries ...any) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for index := 0; index < len(entries); index += 2 {
		node.Content = append(node.Content, scalarNode(entries[index].(string)), entries[index+1].(*yaml.Node))
	}
	return node
}
