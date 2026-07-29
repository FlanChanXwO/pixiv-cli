package clawhubworkflow

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var pinnedAction = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+@[0-9a-f]{40}$`)

// TestPublishWorkflowKeepsTheImmutableReleaseAndSecretBoundary 将 ClawHub
// 发布的信任边界固定为可本地运行的回归测试：发布必须从 Release 交接的不可变
// tag 读取，dry-run 不可获得凭据，且 token 只进入最后的 publish/inspect 步骤。
func TestPublishWorkflowKeepsTheImmutableReleaseAndSecretBoundary(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "publish-clawhub.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse workflow: %v", err)
	}
	if len(document.Content) != 1 {
		t.Fatal("workflow must contain one document")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		t.Fatal("workflow root must be a mapping")
	}
	permissions := mappingValue(t, root, "permissions")
	if permissions.Kind != yaml.MappingNode || len(permissions.Content) != 0 {
		fatalf(t, "workflow must use an empty global permissions mapping")
	}
	on := mappingValue(t, root, "on")
	if mappingValue(t, on, "workflow_run") == nil || mappingValue(t, on, "workflow_dispatch") == nil {
		fatalf(t, "workflow must support trusted workflow_run and manual recovery")
	}
	dispatchInputs := mappingValue(t, mappingValue(t, on, "workflow_dispatch"), "inputs")
	verifyOnly := mappingValue(t, dispatchInputs, "verify_only")
	if scalarValue(t, mappingValue(t, verifyOnly, "type")) != "boolean" || scalarValue(t, mappingValue(t, verifyOnly, "default")) != "false" {
		fatalf(t, "manual recovery must retain the default-off verify_only input")
	}

	jobs := mappingValue(t, root, "jobs")
	publish := mappingValue(t, jobs, "publish")
	jobPermissions := mappingValue(t, publish, "permissions")
	if scalarValue(t, mappingValue(t, jobPermissions, "actions")) != "read" || scalarValue(t, mappingValue(t, jobPermissions, "contents")) != "read" || len(jobPermissions.Content) != 4 {
		fatalf(t, "publish job must retain only actions/content read permissions")
	}
	steps := mappingValue(t, publish, "steps")
	if steps.Kind != yaml.SequenceNode {
		fatalf(t, "publish job must define steps")
	}

	finalIndex := -1
	for index, step := range steps.Content {
		if uses := optionalMappingValue(step, "uses"); uses != nil && !pinnedAction.MatchString(scalarValue(t, uses)) {
			fatalf(t, "step %d uses an unpinned action %q", index, scalarValue(t, uses))
		}
		if strings.Contains(renderNode(t, step), "secrets.CLAWHUB_TOKEN") {
			if scalarValue(t, optionalMappingValue(step, "name")) != "Publish and inspect ClawHub skill" {
				fatalf(t, "CLAWHUB_TOKEN may appear only in the final publish step")
			}
			finalIndex = index
		}
	}
	if finalIndex != len(steps.Content)-1 {
		fatalf(t, "final publish step must be the only secret-bearing step")
	}
	final := steps.Content[finalIndex]
	finalRun := scalarValue(t, mappingValue(t, final, "run"))
	if !strings.Contains(finalRun, "clawhub skill publish skills/pixiv-cli --version \"$SKILL_VERSION\"") || !strings.Contains(finalRun, "clawhub skill verify pixiv-cli --version \"$SKILL_VERSION\"") {
		fatalf(t, "final step must publish and verify the exact versioned product skill")
	}

	dryRun := stepByName(t, steps, "Dry-run the exact tagged skill")
	dryRunText := scalarValue(t, mappingValue(t, dryRun, "run"))
	if strings.Contains(renderNode(t, dryRun), "CLAWHUB_TOKEN") || !strings.Contains(dryRunText, "clawhub skill publish skills/pixiv-cli --version \"$SKILL_VERSION\"") || !strings.Contains(dryRunText, "--dry-run --json") {
		fatalf(t, "dry-run must be credential-free and use the exact versioned product skill")
	}
	verifyOnlyStep := stepByName(t, steps, "Verify an already-published ClawHub skill")
	verifyOnlyRun := scalarValue(t, mappingValue(t, verifyOnlyStep, "run"))
	if strings.Contains(renderNode(t, verifyOnlyStep), "CLAWHUB_TOKEN") || !strings.Contains(verifyOnlyRun, "clawhub skill verify pixiv-cli --version \"$SKILL_VERSION\"") || strings.Contains(verifyOnlyRun, "clawhub skill publish") {
		fatalf(t, "verify-only recovery must be credential-free and must not republish the immutable version")
	}
	install := stepByName(t, steps, "Install pinned ClawHub CLI without credentials")
	if !strings.Contains(scalarValue(t, mappingValue(t, install, "run")), "clawhub@0.23.1") {
		fatalf(t, "ClawHub CLI must stay pinned")
	}
	if !strings.Contains(scalarValue(t, mappingValue(t, install, "run")), "clawhub --cli-version") {
		fatalf(t, "ClawHub CLI version check must use the pinned CLI's supported flag")
	}
	for _, required := range []string{"--source-repo \"$GITHUB_REPOSITORY\"", "--source-ref \"$RELEASE_TAG\"", "--source-commit \"$RELEASE_COMMIT\"", "--source-path skills/pixiv-cli"} {
		if !strings.Contains(dryRunText, required) || !strings.Contains(finalRun, required) {
			fatalf(t, "publish steps must pass immutable source evidence %q", required)
		}
	}
	verify := stepByName(t, steps, "Verify immutable release and exact skill source")
	verifyRun := scalarValue(t, mappingValue(t, verify, "run"))
	for _, required := range []string{"git merge-base --is-ancestor", "releases/tags/$RELEASE_TAG", "skills/pixiv-cli/SKILL.md", "git diff --quiet"} {
		if !strings.Contains(verifyRun, required) {
			fatalf(t, "immutable-source verification is missing %q", required)
		}
	}
}

func mappingValue(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()
	value := optionalMappingValue(node, key)
	if value == nil {
		t.Fatalf("mapping key %q is required", key)
	}
	return value
}

func optionalMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func scalarValue(t *testing.T, node *yaml.Node) string {
	t.Helper()
	if node == nil || node.Kind != yaml.ScalarNode {
		t.Fatalf("expected a scalar node, got %#v", node)
	}
	return node.Value
}

func stepByName(t *testing.T, steps *yaml.Node, name string) *yaml.Node {
	t.Helper()
	for _, step := range steps.Content {
		if nameNode := optionalMappingValue(step, "name"); nameNode != nil && nameNode.Value == name {
			return step
		}
	}
	t.Fatalf("step %q is required", name)
	return nil
}

func renderNode(t *testing.T, node *yaml.Node) string {
	t.Helper()
	content, err := yaml.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func fatalf(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Fatalf(format, args...)
}
