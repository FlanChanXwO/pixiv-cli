package prverificationworkflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("PR verification workflow missing trusted feedback contract %q", required)
		}
	}
	if !strings.Contains(body, "pull-requests: write") {
		t.Fatal("trusted PR feedback path must be able to write PR feedback")
	}
	for _, forbidden := range []string{
		"jq -r '.base.sha'",
		"issues/comments/$comment_id/reactions",
		"issues/comments/$trigger_comment/reactions",
		"reaction_id",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("PR verification workflow contains obsolete trust path %q", forbidden)
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
