package documentation_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestPublicSDKInventoryGolden 校验 v1 公开 SDK 的导出符号清单与 golden 一致，
// 防止已冻结的公开面被意外增删。
func TestPublicSDKInventoryGolden(t *testing.T) {
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
}
