package prverificationworkflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflowDocument struct {
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		Permissions map[string]string `yaml:"permissions"`
	} `yaml:"jobs"`
}

// TestTrustedPRVerificationFeedbackContract 只锁住 /test 的信任边界：
// reaction 走 GraphQL、trusted executor 跟随当前 base branch tip，反馈 job 具备写 PR 的权限。
func TestTrustedPRVerificationFeedbackContract(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "pr-verification.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(workflow)

	for _, required := range []string{
		"gh api graphql",
		"jq -r '.base.ref'",
		"branches/$base_ref_encoded",
		// §5.1/§5.3/§5.4：触发判定与有效命令解析都由单一 trusted policy 负责，
		// 评论 override 完全覆盖 PR 声明，且声明但无效时必须 fail closed。
		"--check-trigger --comment-file",
		"--resolve",
		"OVERRIDE_DECLARED: ${{ steps.resolve.outputs.override_declared }}",
		"fail_trigger \"$RESOLVE_DESC\"",
		"verification_identity()",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("PR verification workflow missing trusted feedback contract %q", required)
		}
	}
	var document workflowDocument
	if err := yaml.Unmarshal(workflow, &document); err != nil {
		t.Fatalf("parse PR verification workflow: %v", err)
	}
	if document.Permissions["pull-requests"] == "write" {
		t.Fatal("workflow-level permissions must not grant pull request write access")
	}
	for _, name := range []string{"dispatch", "aggregate"} {
		job, ok := document.Jobs[name]
		if !ok {
			t.Fatalf("PR verification workflow missing trusted feedback job %q", name)
		}
		if job.Permissions["pull-requests"] != "write" {
			t.Fatalf("trusted feedback job %q must grant pull-requests: write", name)
		}
	}
	for name, job := range document.Jobs {
		if name == "dispatch" || name == "aggregate" {
			continue
		}
		if job.Permissions["pull-requests"] == "write" {
			t.Fatalf("non-feedback job %q must not grant pull-requests: write", name)
		}
	}
	// §28：不导出不参与任何判定的同源标志（未消费的输出了会让读者误以为存在
	// 凭据门禁）。凭据边界本身由下方 TestPRVerificationHoldsNoCredentials 结构性验证。
	for _, forbidden := range []string{
		"same_repo",
		"jq -r '.base.sha'",
		"trimmed=",
		"issues/comments/$comment_id/reactions",
		"issues/comments/$trigger_comment/reactions",
		"reaction_id",
		"subject_id=$(reaction_subject \"$1\") || return 0",
		"-f content=\"$2\" >/dev/null 2>&1 || true",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("PR verification workflow contains obsolete trust path %q", forbidden)
		}
	}
}

func TestDeletedTriggerCommentIsTheOnlyIgnoredReactionLookupError(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "pr-verification.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(workflow)

	// coordinate 与 aggregate 都会清理 trigger reaction；两处都必须只把
	// “触发评论已删除”的 HTTP 404 映射成空 reaction target。
	if got := strings.Count(body, "(HTTP 404)"); got != 2 {
		t.Fatalf("deleted-comment handling markers = %d, want 2", got)
	}
	if got := strings.Count(body, `*"(HTTP 404)"*) return 0 ;;`); got != 2 {
		t.Fatalf("deleted-comment empty-target returns = %d, want 2", got)
	}
	if got := strings.Count(body, `[ -n "$subject_id" ] || return 0`); got != 4 {
		t.Fatalf("reaction helpers do not skip missing deleted-comment targets: %d markers", got)
	}

	for _, forbidden := range []string{
		"removeReaction(input: {subjectId: $subjectId, content: $content}) { subject { id } } }' \\\n              -f subjectId=\"$subject_id\" \\\n              -f content=\"$2\" >/dev/null 2>&1 || true",
		"reaction_subject \"$1\") || return 0",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("reaction cleanup contains broad error suppression %q", forbidden)
		}
	}
}

func TestSupersededRunCancellationFailureIsObservableAndBestEffort(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "pr-verification.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(workflow)

	if strings.Contains(body, `gh api --method POST "repos/$REPO/actions/runs/$old_run/cancel" >/dev/null 2>&1 || true`) {
		t.Fatal("superseded run cancellation must not silently discard failures")
	}
	for _, required := range []string{
		`if cancel_output=$(gh api --method POST "repos/$REPO/actions/runs/$old_run/cancel" 2>&1); then`,
		`failed to cancel superseded verification run %s: %s`,
		`"$old_run" "$cancel_output" >&2`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("superseded run cancellation missing observable best-effort contract %q", required)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

// TestPRVerificationHoldsNoCredentials 覆盖 §28：凭据边界必须是结构性的——整个
// PR verification workflow 不声明任何仓库秘密或发布环境，因此同仓库 PR 与 fork PR
// 都无法通过 trigger 获取受保护凭据。
func TestPRVerificationHoldsNoCredentials(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "pr-verification.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Jobs map[string]map[string]any `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse PR verification workflow: %v", err)
	}
	for name, job := range document.Jobs {
		if _, ok := job["environment"]; ok {
			t.Errorf("job %q must not bind a release environment", name)
		}
		if _, ok := job["secrets"]; ok {
			t.Errorf("job %q must not declare secrets", name)
		}
	}
	// 任何步骤通过 ${{ secrets.* }} 取秘密都会把受保护凭据暴露给 PR 代码。
	if strings.Contains(string(body), "secrets.") {
		t.Error("PR verification must not reference repository secrets")
	}
}
