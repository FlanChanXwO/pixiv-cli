package prverificationworkflow_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// commentBodies 覆盖 §5.1 触发语义的边界：普通评论、合法触发、前置空行、
// 以及“看起来像触发但 trusted parser 会拒绝”的输入。
var commentBodies = []string{
	"LGTM",
	"看起来不错，我先合了。",
	"/test",
	"\n\n/test",
	"   \n\t\n/test",
	"/test\n\nplease run the suite",
	"/test something",
	"please run /test now",
	"```\n/test\n```",
	"",
	"\n\n",
}

// trustedTrigger 调用仓库自己的 tools/prmeta --check-trigger 判定一条评论是否
// 是合法触发。prefilter 的契约必须相对这个真实 parser 定义，而不是在测试里
// 复刻一份简化规则。
func trustedTrigger(t *testing.T, body string) bool {
	t.Helper()

	root := repositoryRoot(t)
	commentPath := filepath.Join(t.TempDir(), "comment.md")
	if err := os.WriteFile(commentPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "run", "./tools/prmeta", "--check-trigger", "--comment-file", commentPath)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("tools/prmeta --check-trigger (%q): %v\n%s", body, err, output)
	}
	return strings.Contains(string(output), "trigger=true")
}

// TestVerificationPrefilterNeverDropsATrustedTrigger 覆盖 R14 的安全性方向：
// job-level prefilter 只是资源过滤器，允许少量 false positive，但绝不允许
// false negative——任何 trusted parser 认可的 /test 都必须通过 prefilter。
func TestVerificationPrefilterNeverDropsATrustedTrigger(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "pr-verification.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(workflow)

	// 先确认 prefilter 确实存在，否则“跳过普通评论”的修复根本没落地。
	if !strings.Contains(body, "contains(github.event.comment.body, '/test')") {
		t.Fatal("PR verification dispatch must cheaply prefilter ordinary comments before allocating a runner")
	}

	// workflow 的 prefilter 语义是“整条评论包含 /test”。逐一验证它对每个
	// trusted 触发都成立，证明不会漏掉合法请求。
	sawTrigger := false
	for _, comment := range commentBodies {
		trigger := trustedTrigger(t, comment)
		passesPrefilter := strings.Contains(comment, "/test")
		if trigger {
			sawTrigger = true
		}
		if trigger && !passesPrefilter {
			t.Errorf("prefilter would drop a trusted trigger: %q", comment)
		}
	}
	if !sawTrigger {
		t.Fatal("fixtures must include at least one trusted trigger, otherwise the property is vacuous")
	}

	// tools/prmeta 必须仍是最终判定者：workflow 不能在 job 条件里自己完成授权。
	if !strings.Contains(body, "go run ./tools/prmeta --check-trigger --comment-file") {
		t.Fatal("tools/prmeta --check-trigger must remain the authoritative trigger decision")
	}
}

// TestVerificationPrefilterSkipsOrdinaryComments 锁定 prefilter 的收益方向：
// 明显不可能触发 /test 的普通评论不得再分配 runner。
func TestVerificationPrefilterSkipsOrdinaryComments(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "pr-verification.yml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(workflow)

	for name, condition := range map[string]string{
		"dispatch job": "Dispatch verification",
	} {
		if !strings.Contains(body, condition) {
			t.Fatalf("%s must still exist", name)
		}
	}
	// dispatch 的 job-level if 必须把 prefilter 与 PR 检查组合在同一个条件里，
	// 这样非 PR 评论与普通 PR 评论都表现为 GitHub 原生 Skipped。
	dispatch := strings.Index(body, "  dispatch:")
	if dispatch < 0 {
		t.Fatal("pr-verification.yml must define a dispatch job")
	}
	tail := body[dispatch:]
	end := strings.Index(tail, "\n    concurrency:")
	if end < 0 {
		t.Fatal("dispatch job must declare concurrency after its condition")
	}
	condition := tail[:end]
	for _, required := range []string{
		"github.event.issue.pull_request",
		"contains(github.event.comment.body, '/test')",
	} {
		if !strings.Contains(condition, required) {
			t.Errorf("dispatch job condition must include %q, got:\n%s", required, condition)
		}
	}
	for _, comment := range []string{"LGTM", "看起来不错，我先合了。", "", "\n\n"} {
		if strings.Contains(comment, "/test") {
			t.Fatalf("fixture %q is not an ordinary comment", comment)
		}
	}
}
