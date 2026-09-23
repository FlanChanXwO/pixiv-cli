package prverificationworkflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPRMetadataValidatesAgainstTheCurrentBaseTip 锁住元数据门的受信执行源：
// pr-metadata 运行的是仓库自己的 tools/prmeta，因此必须 checkout base 分支的
// **当前 tip**。PR 捕获的 .base.sha 可能远落后于受保护分支，在那种提交上
// tools/prmeta 可能根本不存在，门会直接失败（或运行到陈旧策略）。
func TestPRMetadataValidatesAgainstTheCurrentBaseTip(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "pr-metadata.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(workflow)

	if strings.Contains(body, "github.event.pull_request.base.sha") {
		t.Fatal("PR metadata must not execute trusted tooling from the PR-captured base commit")
	}
	for _, required := range []string{
		"jq -r '.base.ref'",
		"branches/$base_ref_encoded",
		"steps.base.outputs.sha",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("PR metadata workflow missing current-base resolution contract %q", required)
		}
	}
}

// TestPRMetadataPreservesSmokeDispatchSafety locks the PR metadata invariants
// that prevent stale heads or stale PR-body events from publishing incorrect
// results, and ensure optional smoke gates use real skipped checks rather than
// synthetic success statuses.
func TestPRMetadataPreservesSmokeDispatchSafety(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "pr-metadata.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(workflow)

	for _, required := range []string{
		`git fetch --no-tags origin "+refs/pull/$PR/head:refs/remotes/pull/$PR/head"`,
		`test "$(git rev-parse "refs/remotes/pull/$PR/head")" = "$HEAD_SHA"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("PR metadata workflow missing exact-head contract %q", required)
		}
	}

	if got := strings.Count(body, "metadata_is_current || exit 0"); got != 2 {
		t.Fatalf("metadata current-body guards = %d, want 2", got)
	}
	if got := strings.Count(body, `if ! cmp -s "$RUNNER_TEMP/event-pr-body-state.md" "$RUNNER_TEMP/current-pr-body-state.md"; then`); got != 1 {
		t.Fatalf("metadata age-state stale-event guards = %d, want 1", got)
	}

	check := strings.Index(body, `create_check "$context" in_progress`)
	dispatch := strings.Index(body, `"repos/$REPO/actions/workflows/$workflow/dispatches"`)
	if check < 0 || dispatch < 0 || check >= dispatch {
		t.Fatal("PR metadata must create the smoke check before dispatching its worker")
	}

	for _, required := range []string{
		`"repos/$REPO/check-runs"`,
		`create_check 'Platform smoke gate' completed skipped`,
		`create_check 'Container smoke gate' completed skipped`,
		`--arg check_run_id "$check_run_id"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("PR metadata workflow missing check-run contract %q", required)
		}
	}

	for _, path := range []string{"platform-smoke.yml", "container-smoke.yml"} {
		worker, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", path))
		if err != nil {
			t.Fatal(err)
		}
		workerBody := string(worker)
		for _, required := range []string{
			"checks: write",
			`"repos/$REPO/check-runs/$CHECK_RUN_ID"`,
			`-f status=completed`,
		} {
			if !strings.Contains(workerBody, required) {
				t.Fatalf("%s missing check-run completion contract %q", path, required)
			}
		}
		if strings.Contains(workerBody, "statuses: write") || strings.Contains(workerBody, `statuses/$HEAD_SHA`) {
			t.Fatalf("%s must not retain commit-status compatibility publication", path)
		}
	}
}

func TestQualityGateUsesJobLevelScopeSkip(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(workflow)
	for _, required := range []string{
		"needs: scope",
		`if: ${{ always() && (needs.scope.result != 'success' || needs.scope.outputs.quality_required == 'true') }}`,
		"Require successful scope classification",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("Quality gate missing job-level skip contract %q", required)
		}
	}
	if strings.Contains(body, "steps.scope.outputs.docs_only") {
		t.Fatal("Quality gate must not emulate a skipped job by running a shell job with skipped steps")
	}
}

func TestVerificationUsesJobLevelSkip(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "pr-verification.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(workflow)
	if !strings.Contains(body, `if: ${{ needs.dispatch.result == 'success' && needs.dispatch.outputs.execute == 'true' }}`) {
		t.Fatal("verification workers must use a real job-level skip when verification is not requested")
	}
	for _, forbidden := range []string{
		`display:"not required"`,
		"Mark verification as not required",
		"matrix.required",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("verification workflow retains fake-skip compatibility path %q", forbidden)
		}
	}
}
