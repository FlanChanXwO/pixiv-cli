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
