// Package checksux_test 锁住 PR Checks 的用户可见契约。
//
// 这里只约束「用户实际看到什么」：Check 名必须回答「检查什么」而非「内部怎么
// 实现」，且 aggregate gate 的失败/跳过必须给出可操作原因。具体 YAML 排版与
// 实现细节留给 Actions 实际运行。
package checksux_test

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

type workflow struct {
	Jobs map[string]struct {
		Name string            `yaml:"name"`
		If   string            `yaml:"if"`
		With map[string]string `yaml:"with"`
	} `yaml:"jobs"`
}

func loadWorkflow(t *testing.T, root, name string) (workflow, string) {
	t.Helper()
	path := filepath.Join(root, ".github", "workflows", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var document workflow
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return document, string(body)
}

// TestEveryJobHasAUserFacingName 保证没有 job 直接暴露内部 job id。
func TestEveryJobHasAUserFacingName(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, ".github", "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		document, _ := loadWorkflow(t, root, entry.Name())
		for jobID, job := range document.Jobs {
			if strings.TrimSpace(job.Name) == "" {
				t.Errorf("%s: job %q has no user-facing name", entry.Name(), jobID)
			}
		}
	}
}

// TestSkippedWorkersDoNotLeakMatrixPlaceholders 覆盖 §11.1/§11.2：
// GitHub 不会为被 skip 的 job 展开 name 中的表达式，因此任何「可能被 skip 且
// name 里带 ${{ matrix.* }}」的 worker 都会把占位符直接显示给用户。
func TestSkippedWorkersDoNotLeakMatrixPlaceholders(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, name := range []string{"platform-smoke.yml", "container-smoke.yml", "pr-verification.yml"} {
		document, _ := loadWorkflow(t, root, name)
		for jobID, job := range document.Jobs {
			if !strings.Contains(job.Name, "${{") {
				continue
			}
			condition := strings.TrimSpace(job.If)
			if condition == "" {
				// 无条件 = 必然运行，名字会被正常求值。
				continue
			}
			if strings.Contains(condition, "always()") {
				// always() 是「必然运行」的显式写法；配合 step 级跳过，
				// 名称里的 matrix 表达式会被求值，不会暴露占位符。
				continue
			}
			// 其余带条件且依赖 matrix 的名字在 skip 时会原样显示占位符。
			t.Errorf("%s: job %q has a skipping condition %q and a matrix-based name %q; use always() with step-level skips",
				name, jobID, condition, job.Name)
		}
	}
}

// TestContainerSmokeUsesAStaticSetupName 保证 setup job 不再暴露内部实现词。
func TestContainerSmokeUsesAStaticSetupName(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	document, body := loadWorkflow(t, root, "container-smoke.yml")
	if strings.Contains(body, "name: Resolve container matrix") {
		t.Error("container setup must use a user-facing name, not \"Resolve container matrix\"")
	}
	if _, ok := document.Jobs["resolve_platforms"]; ok {
		if !strings.Contains(body, "name: Container setup") {
			t.Error("container setup job must be named \"Container setup\"")
		}
	}
	if !strings.Contains(body, "display") {
		t.Error("container workers must take their display name from the resolver, not from the matrix tuple")
	}
}

// TestSetupJobsUseUserFacingNames 覆盖 §11.3：setup job 回答「检查什么」。
func TestSetupJobsUseUserFacingNames(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for name, want := range map[string]string{
		"platform-smoke.yml":  "Platform setup",
		"container-smoke.yml": "Container setup",
	} {
		_, body := loadWorkflow(t, root, name)
		if !strings.Contains(body, "name: "+want) {
			t.Errorf("%s: setup job must be named %q", name, want)
		}
		if strings.Contains(body, "name: Resolve platform matrix") || strings.Contains(body, "name: Resolve container matrix") {
			t.Errorf("%s: setup job must not use internal implementation wording", name)
		}
	}
}

// TestBrowserEvidenceUsesSectionSeparator 覆盖 §11.3 的浏览器 Check 命名。
func TestBrowserEvidenceUsesSectionSeparator(t *testing.T) {
	t.Parallel()

	_, body := loadWorkflow(t, repositoryRoot(t), "browser-evidence.yml")
	// 平台集合由 ci/platforms.json 决定，因此名称取自 resolver 的 display 字段。
	for _, want := range []string{
		"name: Browser provider · ${{ matrix.display }}",
		"name: Firefox profile · ${{ matrix.display }}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("browser-evidence.yml: expected check name %q", want)
		}
	}
}

// TestAggregateGatesExplainFailuresAndSkips 覆盖 §12.1/§12.2。
func TestAggregateGatesExplainFailuresAndSkips(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	cases := []struct {
		workflow string
		required []string
	}{
		{
			workflow: "platform-smoke.yml",
			required: []string{
				"Platform smoke failed on one or more required platforms.",
				"Platform configuration could not be resolved.",
				"Documentation-only change; platform smoke is not required.",
			},
		},
		{
			workflow: "container-smoke.yml",
			required: []string{
				"Container smoke failed on one or more required platforms.",
				"Container smoke is not required for this change.",
			},
		},
		{
			workflow: "ci.yml",
			required: []string{
				"Required quality checks did not complete successfully.",
			},
		},
	}
	for _, tc := range cases {
		_, body := loadWorkflow(t, root, tc.workflow)
		for _, want := range tc.required {
			if !strings.Contains(body, want) {
				t.Errorf("%s: aggregate gate must explain its outcome with %q", tc.workflow, want)
			}
		}
	}
}
