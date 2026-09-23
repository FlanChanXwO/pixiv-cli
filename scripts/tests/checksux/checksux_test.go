// Package checksux_test 锁住 PR Checks 的用户可见契约。
//
// 这里只约束「用户实际看到什么」：Check 名必须回答「检查什么」而非「内部怎么
// 实现」，且 aggregate gate 的失败/跳过必须给出可操作原因。具体 YAML 排版与
// 实现细节留给 Actions 实际运行。
package checksux_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current directory: %v", err)
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && info.Mode().IsRegular() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

type workflow struct {
	Jobs map[string]struct {
		Name string            `yaml:"name"`
		If   string            `yaml:"if"`
		With map[string]string `yaml:"with"`
	} `yaml:"jobs"`
}

func loadWorkflow(t *testing.T, root, name string) (workflow, string) {
	t.Helper()
	path := filepath.Join(root, ".github", "workflows", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var document workflow
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return document, string(body)
}

// TestEveryJobHasAUserFacingName 保证没有 job 直接暴露内部 job id。
func TestEveryJobHasAUserFacingName(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, ".github", "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		document, _ := loadWorkflow(t, root, entry.Name())
		for jobID, job := range document.Jobs {
			if strings.TrimSpace(job.Name) == "" {
				t.Errorf("%s: job %q has no user-facing name", entry.Name(), jobID)
			}
		}
	}
}

// TestSkippedWorkersDoNotLeakMatrixPlaceholders 覆盖 §11.1/§11.2：
// GitHub 不会为被 skip 的 job 展开 name 中的表达式，因此任何「可能被 skip 且
// name 里带 ${{ matrix.* }}」的 worker 都会把占位符直接显示给用户。
func TestSkippedWorkersDoNotLeakMatrixPlaceholders(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, name := range []string{"platform-smoke.yml", "container-smoke.yml", "pr-verification.yml"} {
		document, _ := loadWorkflow(t, root, name)
		for jobID, job := range document.Jobs {
			if !strings.Contains(job.Name, "${{") {
				continue
			}
			condition := strings.TrimSpace(job.If)
			if condition == "" {
				// 无条件 = 必然运行，名字会被正常求值。
				continue
			}
			if strings.Contains(condition, "always()") {
				// always() 是「必然运行」的显式写法；配合 step 级跳过，
				// 名称里的 matrix 表达式会被求值，不会暴露占位符。
				continue
			}
			// 其余带条件且依赖 matrix 的名字在 skip 时会原样显示占位符。
			t.Errorf("%s: job %q has a skipping condition %q and a matrix-based name %q; use always() with step-level skips",
				name, jobID, condition, job.Name)
		}
	}
}

// TestAggregateGatesExplainFailuresAndSkips 覆盖 §12.1/§12.2。
func TestAggregateGatesExplainFailuresAndSkips(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	_, qualityBody := loadWorkflow(t, root, "ci.yml")
	if !strings.Contains(qualityBody, "Required quality checks did not complete successfully.") {
		t.Errorf("ci.yml: quality gate must explain failures")
	}

	_, metadataBody := loadWorkflow(t, root, "pr-metadata.yml")
	for _, want := range []string{
		"Documentation-only change; platform smoke is not required.",
		"Container smoke is not required for this change.",
	} {
		if !strings.Contains(metadataBody, want) {
			t.Errorf("pr-metadata.yml: aggregate gate must explain its outcome with %q", want)
		}
	}

	for name, want := range map[string]string{
		"platform-smoke.yml":  "Platform smoke failed on one or more required platforms.",
		"container-smoke.yml": "Container smoke failed on one or more required platforms.",
	} {
		_, body := loadWorkflow(t, root, name)
		if !strings.Contains(body, want) {
			t.Errorf("%s: aggregate gate must explain failures with %q", name, want)
		}
	}
}

// TestTrustedPolicyOwnsSmokeDispatch 保证不受信的 pull_request 只运行 Quality；
// 高权限 worker dispatch 与稳定 smoke status 只存在于 pull_request_target 策略流。
func TestTrustedPolicyOwnsSmokeDispatch(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	document, qualityBody := loadWorkflow(t, root, "ci.yml")
	if _, ok := document.Jobs["quality_gate"]; !ok {
		t.Error("ci.yml must expose PR job \"quality_gate\"")
	}
	if len(document.Jobs) != 1 {
		t.Errorf("ci.yml exposes %d jobs, want only the untrusted Quality gate", len(document.Jobs))
	}
	for _, forbidden := range []string{"actions: write", "/dispatches"} {
		if strings.Contains(qualityBody, forbidden) {
			t.Errorf("ci.yml must not contain trusted smoke capability %q", forbidden)
		}
	}

	_, metadataBody := loadWorkflow(t, root, "pr-metadata.yml")
	for _, required := range []string{
		"pull_request_target:",
		"actions: write",
		"statuses: write",
		"group: pr-gates-${{ github.event.pull_request.number }}-${{ github.event.action == 'edited' && github.event.changes.base == null && 'metadata' || 'head' }}",
		"cancel-in-progress: true",
		"id: smoke_head",
		"steps.smoke_head.outcome == 'success'",
		"steps.smoke_head.outcome == 'failure'",
		"'Platform smoke gate'",
		"'Container smoke gate'",
		"platform-smoke.yml",
		"container-smoke.yml",
	} {
		if !strings.Contains(metadataBody, required) {
			t.Errorf("pr-metadata.yml must contain trusted smoke contract %q", required)
		}
	}
	if strings.Count(metadataBody, "PR body changed since this event; skipping stale metadata") != 2 {
		t.Error("pr-metadata.yml must reject stale PR-body snapshots before status and state publication")
	}
	if strings.Count(metadataBody, `jq -j '.body // ""'`) != 2 {
		t.Error("pr-metadata.yml must re-read the current PR body at both metadata publication boundaries")
	}
	stateStart := strings.Index(metadataBody, "name: Maintain invalid-PR age state")
	stateEnd := strings.Index(metadataBody, "name: Require metadata gates")
	if stateStart < 0 || stateEnd <= stateStart {
		t.Fatal("pr-metadata.yml must contain invalid-PR state maintenance before the final metadata gate")
	}
	stateBlock := metadataBody[stateStart:stateEnd]
	if !strings.Contains(stateBlock, "if: ${{ !cancelled() && steps.policy.outcome != 'skipped' }}") {
		t.Error("invalid-PR state maintenance must survive unrelated smoke failures")
	}
	if strings.Contains(metadataBody, "return_run_details") {
		t.Error("pr-metadata.yml must use the 2026-03-10 dispatch contract without return_run_details")
	}
	if strings.Count(metadataBody, "github.event.action != 'edited' || github.event.changes.base != null") < 4 {
		t.Error("pr-metadata.yml must rerun smoke coordination when an edited event retargets the PR base branch")
	}
	if !strings.Contains(metadataBody, "Failed to publish smoke failure status for $context.") {
		t.Error("pr-metadata.yml must attempt both smoke failure statuses even if one status API call fails")
	}
	headStart := strings.Index(metadataBody, "name: Fetch pull request head for trusted smoke classification")
	headEnd := strings.Index(metadataBody, "name: Classify smoke scope with trusted policy")
	if headStart < 0 || headEnd <= headStart {
		t.Fatal("pr-metadata.yml must contain the trusted PR-head fetch before smoke classification")
	}
	headBlock := metadataBody[headStart:headEnd]
	if !strings.Contains(headBlock, "continue-on-error: true") {
		t.Error("trusted PR-head fetch must preserve a terminal smoke failure path")
	}
	pending := strings.Index(metadataBody, `post_status pending "$context"`)
	dispatch := strings.Index(metadataBody, `"repos/$REPO/actions/workflows/$workflow/dispatches"`)
	if pending < 0 || dispatch < 0 || pending >= dispatch {
		t.Error("smoke pending status must be published before worker dispatch")
	}

	for _, name := range []string{"platform-smoke.yml", "container-smoke.yml"} {
		_, body := loadWorkflow(t, root, name)
		if !strings.Contains(body, "workflow_dispatch:") {
			t.Errorf("%s must remain manually dispatchable for hidden workers", name)
		}
		for _, trigger := range []string{"  pull_request:", "  push:"} {
			if strings.Contains(body, trigger) {
				t.Errorf("%s must not expose worker jobs through %q", name, strings.TrimSpace(trigger))
			}
		}
		if strings.Count(body, "ref: ${{ inputs.pr_number != ''") != 1 {
			t.Errorf("%s must checkout untrusted PR code only in the matrix worker job", name)
		}
		if !strings.Contains(body, `test "$(git rev-parse HEAD)" = "$HEAD_SHA"`) {
			t.Errorf("%s must verify that the matrix worker tested the dispatched head SHA", name)
		}
	}

	for name, required := range map[string][]string{
		"platform-smoke.yml": {
			"name: Publish platform smoke result",
			"statuses: write",
			"'Platform smoke gate'",
		},
		"container-smoke.yml": {
			"name: Publish container smoke result",
			"statuses: write",
			"'Container smoke gate'",
		},
	} {
		_, body := loadWorkflow(t, root, name)
		for _, want := range required {
			if !strings.Contains(body, want) {
				t.Errorf("%s must contain trusted result publisher contract %q", name, want)
			}
		}
	}
}
