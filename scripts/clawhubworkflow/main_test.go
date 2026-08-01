package clawhubworkflow

import (
	"encoding/json"
	"os"
	"os/exec"
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

	secretSteps := make(map[string]int)
	for index, step := range steps.Content {
		if uses := optionalMappingValue(step, "uses"); uses != nil && !pinnedAction.MatchString(scalarValue(t, uses)) {
			fatalf(t, "step %d uses an unpinned action %q", index, scalarValue(t, uses))
		}
		if strings.Contains(renderNode(t, step), "secrets.CLAWHUB_TOKEN") {
			secretSteps[scalarValue(t, optionalMappingValue(step, "name"))] = index
		}
	}
	finalIndex, ok := secretSteps["Publish and inspect ClawHub skill"]
	if !ok || finalIndex != len(steps.Content)-1 || len(secretSteps) != 2 {
		fatalf(t, "publish must be final and only publish/verify-only steps may read CLAWHUB_TOKEN")
	}
	final := steps.Content[finalIndex]
	finalRun := scalarValue(t, mappingValue(t, final, "run"))
	if !strings.Contains(finalRun, "clawhub skill publish skills/pixiv-cli --version \"$SKILL_VERSION\"") || !strings.Contains(finalRun, "clawhub skill verify pixiv-cli --version \"$SKILL_VERSION\"") {
		fatalf(t, "final step must publish and verify the exact versioned product skill")
	}
	if !strings.Contains(finalRun, "staticScanClean") || !strings.Contains(finalRun, "aggregateSecurityPending") || !strings.Contains(finalRun, "else if (pendingAggregationOnly)") || !strings.Contains(finalRun, "ClawHub aggregate security scan pending") {
		fatalf(t, "publish step must make clean static scanning and pending aggregation an explicit verification branch")
	}

	dryRun := stepByName(t, steps, "Dry-run the exact tagged skill")
	dryRunText := scalarValue(t, mappingValue(t, dryRun, "run"))
	if scalarValue(t, mappingValue(t, dryRun, "id")) != "dry_run" || strings.Contains(renderNode(t, dryRun), "CLAWHUB_TOKEN") || !strings.Contains(dryRunText, "clawhub skill publish skills/pixiv-cli --version \"$SKILL_VERSION\"") || !strings.Contains(dryRunText, "--dry-run --json") || !strings.Contains(dryRunText, "fingerprint=${result.fingerprint}") {
		fatalf(t, "dry-run must be credential-free and use the exact versioned product skill")
	}
	verifyOnlyStep := stepByName(t, steps, "Verify an already-published ClawHub skill")
	verifyOnlyRun := scalarValue(t, mappingValue(t, verifyOnlyStep, "run"))
	if !strings.Contains(renderNode(t, verifyOnlyStep), "CLAWHUB_TOKEN") || !strings.Contains(verifyOnlyRun, "clawhub --no-input login --token \"$CLAWHUB_TOKEN\"") || !strings.Contains(verifyOnlyRun, "clawhub skill verify pixiv-cli --version \"$SKILL_VERSION\" > \"$RUNNER_TEMP/clawhub-verify.json\" || verify_exit=$?") || !strings.Contains(verifyOnlyRun, "securityIsClean") || !strings.Contains(verifyOnlyRun, "cardPendingOnly") || !strings.Contains(verifyOnlyRun, "verify.artifact?.sourceFingerprint === fingerprint") || strings.Contains(verifyOnlyRun, "aggregateSecurityPending") || strings.Contains(verifyOnlyRun, "clawhub skill publish") {
		fatalf(t, "verify-only recovery must require a completed clean scan and must not republish the immutable version")
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

// TestPublishWorkflowAcceptsOnlyDocumentedPendingAggregation 验证工作流内嵌的
// Node 校验器本身。ClawHub 的 aggregate security 是异步信号：只有静态扫描已
// clean、reason 位于封闭白名单且命令以状态 1 退出时，发布后检查才可暂时通过。
func TestPublishWorkflowAcceptsOnlyDocumentedPendingAggregation(t *testing.T) {
	const version = "v0.10.0"
	const fingerprint = "test-fingerprint"
	publish := map[string]any{
		"slug":        "pixiv-cli",
		"version":     version,
		"fingerprint": fingerprint,
	}
	pending := func(reasons []string, staticScan string) map[string]any {
		return map[string]any{
			"schema":       "clawhub.skill.verify.v1",
			"slug":         "pixiv-cli",
			"version":      version,
			"resolvedFrom": "version",
			"artifact":     map[string]any{"sourceFingerprint": fingerprint},
			"security": map[string]any{
				"passed": false,
				"status": "pending",
				"signals": map[string]any{
					"staticScan": map[string]any{"status": staticScan},
				},
			},
			"ok":       false,
			"decision": "fail",
			"reasons":  reasons,
		}
	}

	publishScript := workflowNodeScript(t, "Publish and inspect ClawHub skill")
	// card.missing 与 pending aggregate 同时出现时，必须走 aggregate 分支而不是
	// 要求不可能成立的 clean aggregate security。
	runWorkflowNodeVerification(t, publishScript, publish, pending([]string{"card.missing"}, "clean"), version, fingerprint, "1", true)
	runWorkflowNodeVerification(t, publishScript, publish, pending([]string{"security.pending"}, "dirty"), version, fingerprint, "1", false)
	runWorkflowNodeVerification(t, publishScript, publish, pending([]string{"unexpected.reason"}, "clean"), version, fingerprint, "1", false)
	runWorkflowNodeVerification(t, publishScript, publish, pending([]string{"security.pending"}, "clean"), version, fingerprint, "0", false)

	// verify_only 是最终严格检查：aggregate security 尚未完成时不得将本次发布
	// 标记为已复核。
	verifyOnlyScript := workflowNodeScript(t, "Verify an already-published ClawHub skill")
	runWorkflowNodeVerification(t, verifyOnlyScript, nil, pending([]string{"security.pending"}, "clean"), version, fingerprint, "1", false)
}

func workflowNodeScript(t *testing.T, stepName string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "publish-clawhub.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse workflow: %v", err)
	}
	steps := mappingValue(t, mappingValue(t, mappingValue(t, document.Content[0], "jobs"), "publish"), "steps")
	run := scalarValue(t, mappingValue(t, stepByName(t, steps, stepName), "run"))
	_, script, found := strings.Cut(run, "<<'NODE'\n")
	if !found {
		t.Fatalf("step %q must contain a Node verifier", stepName)
	}
	script, _, found = strings.Cut(script, "\nNODE\n")
	if !found {
		t.Fatalf("step %q Node verifier must close its heredoc", stepName)
	}
	return script
}

func runWorkflowNodeVerification(t *testing.T, script string, publish, verify map[string]any, version, fingerprint, exitCode string, wantSuccess bool) {
	t.Helper()
	directory := t.TempDir()
	verifyPath := filepath.Join(directory, "verify.json")
	writeWorkflowJSON(t, verifyPath, verify)
	arguments := []string{"-", verifyPath, version, fingerprint, exitCode}
	if publish != nil {
		publishPath := filepath.Join(directory, "publish.json")
		writeWorkflowJSON(t, publishPath, publish)
		arguments = []string{"-", publishPath, verifyPath, version, fingerprint, exitCode}
	}
	command := exec.Command("node", arguments...)
	command.Stdin = strings.NewReader(script)
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("workflow verifier unexpectedly rejected result: %v\n%s", err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("workflow verifier unexpectedly accepted result: %s", output)
	}
}

func writeWorkflowJSON(t *testing.T, path string, value map[string]any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
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
