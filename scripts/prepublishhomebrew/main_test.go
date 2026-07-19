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

// Linux hosted runner 的 Linuxbrew Resource staging 会在 Homebrew 内部 cleanup 触发
// EINVAL；Linux 验证必须转入固定镜像的短生命周期容器，macOS 仍保留原生 Homebrew。
func TestWorkflowUsesLinuxOnlyContainerizedHomebrewVerification(t *testing.T) {
	root := prepublishWorkflowRoot(t)
	step := runStepWith(t, job(t, root, "verify_homebrew_formula"), "brew install")
	run := mappingValue(t, step, "run").Value
	for _, want := range []string{
		`docker run --rm`,
		`homebrew/brew@sha256:b0072bfdebf5934ae24b93b44a1928a88057399b3283ffa0177bb86084fdedfd`,
		`--entrypoint bash`,
		`type=bind,src=$staging_dir,dst=/staging-formula,readonly`,
		`--env RELEASE_TAG`,
		`HOMEBREW_NO_AUTO_UPDATE=1`,
		`HOMEBREW_NO_ENV_HINTS=1`,
		`brew install --formula "$staging_tap/$formula_name"`,
		`pixiv version --json`,
	} {
		if !strings.Contains(run, want) {
			t.Fatalf("prepublish Linux container verification missing %q", want)
		}
	}
	for _, forbidden := range []string{"--build-from-source", "--debug-symbols", "--keep-tmp", "HOMEBREW_TEMP"} {
		if strings.Contains(run, forbidden) {
			t.Fatalf("prepublish Linux container verification retains obsolete %q", forbidden)
		}
	}
	linuxBranch, macOSBranch, ok := strings.Cut(run, "\nelse\n")
	if !ok || strings.Contains(linuxBranch, "brew trust --tap") || strings.Contains(linuxBranch, "python3 -c") {
		t.Fatal("fixed Linux Homebrew 4.6 container must not call unavailable brew trust or Python")
	}
	if !strings.Contains(linuxBranch, `brew ruby -rjson -e`) {
		t.Fatal("fixed Linux container must compare the JSON version with Ruby standard JSON")
	}
	if !strings.Contains(macOSBranch, "brew trust --tap \"$staging_tap\"") {
		t.Fatal("macOS native Homebrew must retain explicit staging-tap trust")
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
			name: "uses a mutable Linux container image",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRun(t, runStepWith(t, job(t, root, "verify_homebrew_formula"), "docker run --rm"), "homebrew/brew@sha256:b0072bfdebf5934ae24b93b44a1928a88057399b3283ffa0177bb86084fdedfd", "homebrew/brew:latest")
			},
		},
		{
			name: "makes the Linux staging mount writable",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRun(t, runStepWith(t, job(t, root, "verify_homebrew_formula"), "docker run --rm"), ",readonly", "")
			},
		},
		{
			name: "omits release tag from Linux container",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRun(t, runStepWith(t, job(t, root, "verify_homebrew_formula"), "docker run --rm"), "--env RELEASE_TAG", "")
			},
		},
		{
			name: "uses obsolete Linux source flags",
			mutate: func(t *testing.T, root *yaml.Node) {
				replaceRun(t, runStepWith(t, job(t, root, "verify_homebrew_formula"), "docker run --rm"), "brew install --formula", "brew install --debug-symbols --formula")
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
