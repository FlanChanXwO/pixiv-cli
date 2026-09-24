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
// results. Required smoke contexts belong to real GitHub Actions jobs so an
// unnecessary smoke gate is represented by a native job-level skip; trusted
// pull_request_target coordination only owns distinct worker Check Runs.
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
		`dispatch_worker platform-smoke.yml 'Platform smoke worker'`,
		`dispatch_worker container-smoke.yml 'Container smoke worker'`,
		`--arg check_run_id "$check_run_id"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("PR metadata workflow missing check-run contract %q", required)
		}
	}
	for _, legacy := range []string{
		"'Platform smoke gate'",
		"'Container smoke gate'",
		`create_check 'Platform smoke'`,
		`create_check 'Container smoke'`,
	} {
		if strings.Contains(body, legacy) {
			t.Fatalf("PR metadata workflow publishes a required smoke context manually %q", legacy)
		}
	}

	for _, required := range []string{
		"platform_smoke:",
		"name: Platform smoke",
		`needs.validate.outputs.platform_required == 'true'`,
		`WORKER_CHECK: Platform smoke worker`,
		"container_smoke:",
		"name: Container smoke",
		`needs.validate.outputs.container_required == 'true'`,
		`WORKER_CHECK: Container smoke worker`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("PR metadata workflow missing required smoke job contract %q", required)
		}
	}
	for _, forbidden := range []string{"Platform smoke metadata refresh", "Container smoke metadata refresh"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("required smoke job name must be static; found %q", forbidden)
		}
	}
	if got := strings.Count(body, `github.event.action != 'edited' || github.event.changes.base != null`); got != 1 {
		t.Fatalf("body-edit exclusion count = %d, want 1 only on worker dispatch", got)
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
	for _, bootstrap := range []string{
		"smoke_context_bootstrap_platform:",
		"smoke_context_bootstrap_container:",
		"Required smoke context bootstrap",
	} {
		if strings.Contains(body, bootstrap) {
			t.Fatalf("temporary smoke-context bootstrap remains after migration: %q", bootstrap)
		}
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

// TestQualityScopeClassificationUsesTrustedBase 覆盖 R6：Quality 的 skip 判定
// 必须与 pr-metadata 的 smoke classification 使用同一信任模型。PR 能修改
// scripts/classify-change-scope.sh 与 .github/ci-change-scope.gitignore，因此
// 分类绝不能从 PR 自己的 checkout 执行，否则 PR 可以决定自己的 Quality 是否运行。
func TestQualityScopeClassificationUsesTrustedBase(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(workflow)

	// 必须解析受保护 base branch 的当前 tip，而不是 PR 捕获的 .base.sha。
	if strings.Contains(body, "github.event.pull_request.base.sha") {
		t.Fatal("Quality scope must not classify against the PR-captured base commit")
	}
	for _, required := range []string{
		"jq -r '.base.ref'",
		"branches/$base_ref_encoded",
		"steps.base.outputs.sha",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("Quality scope workflow missing current-base resolution contract %q", required)
		}
	}

	// 分类步骤必须运行 base tip 中的 trusted 脚本。
	if !strings.Contains(body, "bash ./scripts/classify-change-scope.sh") {
		t.Fatal("Quality scope must delegate to the repository classify-change-scope.sh")
	}
	classify := strings.Index(body, "bash ./scripts/classify-change-scope.sh")
	baseCheckout := strings.Index(body, "ref: ${{ github.event_name == 'pull_request' && steps.base.outputs.sha || github.sha }}")
	if baseCheckout < 0 || classify < 0 || baseCheckout > classify {
		t.Fatal("Quality scope must check out the trusted base tip before running the classifier")
	}

	// 必须 fetch 精确的 PR HEAD，保证被分类的 diff 就是被测试的 diff。
	for _, required := range []string{
		`git fetch --no-tags origin "+refs/pull/$PR/head:refs/remotes/pull/$PR/head"`,
		`test "$(git rev-parse "refs/remotes/pull/$PR/head")" = "$HEAD_SHA"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("Quality scope workflow missing exact-head contract %q", required)
		}
	}

	// classifier 失败必须 fail closed：不得退化为 skip。
	for _, required := range []string{
		"Require successful scope classification",
		`needs.scope.result != 'success'`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("Quality scope workflow must fail closed on classifier failure (%q missing)", required)
		}
	}
	if strings.Contains(body, "|| true") && strings.Contains(body, "classify-change-scope.sh") {
		t.Fatal("Quality scope must not swallow classifier failures")
	}
}

// TestQualityGateDoesNotRunOnTagPush 覆盖 R15：stable/prerelease tag 由 Release
// 独占发布门禁。同一个 tag push 不应再并行启动一遍 Quality gate 跑重复的
// go test / race / vet / package，那是让 tag 承担两次同一套验证。
func TestQualityGateDoesNotRunOnTagPush(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	qualityBody, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	quality := string(qualityBody)
	if !strings.Contains(quality, "pull_request:") {
		t.Fatal("Quality gate must still serve pull requests")
	}
	for _, forbidden := range []string{"push:", "tags:"} {
		if strings.Contains(quality, forbidden) {
			t.Fatalf("Quality gate must not respond to %q; Release owns tag publication", forbidden)
		}
	}

	releaseBody, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	release := string(releaseBody)
	for _, required := range []string{"push:", "tags:", "'v[0-9]*'"} {
		if !strings.Contains(release, required) {
			t.Fatalf("Release must own the tag push trigger (%q missing)", required)
		}
	}
}
