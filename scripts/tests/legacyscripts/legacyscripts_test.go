// Package legacyscripts_test 锁定 §25.2/§25.3：旧 E2E wrapper 已删除，且不再有
// 第二套 orchestration 入口。
//
// 保留的是底层能力本身（`go test ./e2e -run TestReal...`、离线 e2e 契约测试），
// 删除的只是只服务于旧入口的 wrapper 与它们的专属测试。
package legacyscripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// TestLegacyE2EWrappersAreRemoved 覆盖 §25.2 的「至少删除」清单及其专属 glue。
func TestLegacyE2EWrappersAreRemoved(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, relative := range []string{
		"scripts/test-e2e.sh",
		"scripts/test-reverse-search-e2e.sh",
		// 只测试旧 wrapper 行为的测试必须随 wrapper 一起删除。
		"e2e/test_e2e_script_test.go",
		"e2e/reverse_search_e2e_script_test.go",
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err == nil {
			t.Errorf("legacy E2E entrypoint must be deleted: %s", relative)
		}
	}
}

// TestNoWorkflowOrDocInvokesLegacyE2EWrappers 覆盖 §25.3：不得同时存在两套
// orchestration 入口，因此 CI、脚本与文档都不能再引用旧 wrapper。
func TestNoWorkflowOrDocInvokesLegacyE2EWrappers(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, relative := range []string{
		"scripts",
		"docs",
		".agents",
		"README.md",
		"README.zh-CN.md",
		"CONTRIBUTING.md",
		"CONTRIBUTING.zh-CN.md",
	} {
		path := filepath.Join(root, relative)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			assertNoLegacyReference(t, path, relative)
			continue
		}
		err = filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".md") && !strings.HasSuffix(entry.Name(), ".sh") &&
				!strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), ".yml") {
				return nil
			}
			// 本测试必须写出旧路径才能完成检查。
			if entry.Name() == "legacyscripts_test.go" {
				return nil
			}
			assertNoLegacyReference(t, current, current)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func assertNoLegacyReference(t *testing.T, path, label string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"test-e2e.sh", "test-reverse-search-e2e.sh"} {
		if strings.Contains(string(body), needle) {
			t.Errorf("%s must not reference the deleted legacy wrapper %q", label, needle)
		}
	}
}
