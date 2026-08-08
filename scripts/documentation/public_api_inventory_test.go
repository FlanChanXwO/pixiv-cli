package documentation_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublicAPISDKInventoryGolden 校验 v1 公开 SDK 的导出符号清单与 golden 一致，
// 防止已冻结的公开面被意外增删，并显式拒绝 RC-1 已删除的旧错误命名。
func TestPublicAPISDKInventoryGolden(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	cmd := exec.Command("go", "run", filepath.Join(repositoryRoot, "scripts", "publicapi"),
		"-dir", repositoryRoot,
		"-check",
		"-golden", filepath.Join(repositoryRoot, "docs/maintainers/public-api-inventory.md"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("public API inventory drifted: %v\n%s", err, output)
	}

	golden, err := os.ReadFile(filepath.Join(repositoryRoot, "docs/maintainers/public-api-inventory.md"))
	if err != nil {
		t.Fatalf("read public API inventory: %v", err)
	}
	for line := range strings.SplitSeq(string(golden), "\n") {
		if strings.HasPrefix(line, "- Code") || line == "- Error.Code" || line == "- CodeOf" || line == "- IsCode" {
			t.Fatalf("public API inventory contains removed error symbol: %q", line)
		}
	}
}
