package prverificationworkflow_test

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
