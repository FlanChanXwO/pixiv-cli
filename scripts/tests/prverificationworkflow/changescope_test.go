package prverificationworkflow_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runScopeClassifier 在临时仓库里执行仓库自己的 scripts/classify-change-scope.sh，
// 对一个“只改动 targetPath”的提交做真实分类。判定必须由真实 policy 文件驱动，
// 而不是在测试里复刻规则，否则规则被改错时测试仍会通过。
func runScopeClassifier(t *testing.T, targetPath string) map[string]string {
	t.Helper()

	root := repositoryRoot(t)
	dir := t.TempDir()
	// classifier 在临时仓库之外单独存一份被执行的副本：真实流水线里它来自
	// trusted base checkout，本测试也必须与“被分类的 PR 变更”解耦，否则
	// 修改 classifier 自身的用例会覆盖掉正在执行的脚本。
	trustedClassifier := filepath.Join(t.TempDir(), "classify-change-scope.sh")
	if body, err := os.ReadFile(filepath.Join(root, "scripts", "classify-change-scope.sh")); err != nil {
		t.Fatalf("read trusted classifier: %v", err)
	} else if err := os.WriteFile(trustedClassifier, body, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFile := func(relative string) {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		target := filepath.Join(dir, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, body, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=scope", "GIT_AUTHOR_EMAIL=scope@example.invalid",
			"GIT_COMMITTER_NAME=scope", "GIT_COMMITTER_EMAIL=scope@example.invalid",
		)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
		return string(output)
	}

	git("init", "-q", ".")
	git("config", "user.name", "scope")
	git("config", "user.email", "scope@example.invalid")

	// 先铺基线 fixture（内容不重要，只需让规则能命中路径）。
	for _, fixture := range []string{
		".github/workflows/pr-metadata.yml",
		".github/workflows/platform-smoke.yml",
		".github/workflows/container-smoke.yml",
		"scripts/classify-change-scope.sh",
		"ci/platforms.json",
		"tools/platformmatrix/main.go",
		"README.md",
		"internal/cli/root.go",
	} {
		target := filepath.Join(dir, fixture)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("baseline\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 再覆盖上真实 policy 与 classifier，确保判定由仓库自身规则驱动。
	copyFile(".github/ci-change-scope.gitignore")
	copyFile("scripts/classify-change-scope.sh")
	git("add", "-A")
	git("commit", "-qm", "base")

	target := filepath.Join(dir, targetPath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-qm", "change")

	command := exec.Command("bash", trustedClassifier, "--base", "HEAD~1", "--head", "HEAD")
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("classify-change-scope.sh: %v\n%s", err, output)
	}
	scope := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		scope[key] = value
	}
	return scope
}

// TestSmokeControllerChangesRequireFullSmoke 覆盖 R9：pr-metadata.yml 是 smoke
// controller——它决定 smoke classification、dispatch 两个 worker、并持有
// Platform/Container required gate。因此改动它必须真的跑 full smoke，否则
// controller 自身的破坏性修改会被判成“只需 Quality”而跳过全部 smoke。
func TestSmokeControllerChangesRequireFullSmoke(t *testing.T) {
	t.Parallel()

	scope := runScopeClassifier(t, ".github/workflows/pr-metadata.yml")
	for _, key := range []string{"quality_required", "platform_required", "container_required"} {
		if scope[key] != "true" {
			t.Errorf("changing pr-metadata.yml must require full smoke: %s = %q, want \"true\" (scope=%v)", key, scope[key], scope)
		}
	}
}

// TestTrustedSmokeOwnerScopesRemainCorrect 锁定审查结论：其余 trusted smoke owner
// 只需停在它们真正拥有的最小 scope，不扩大到所有 CI 文件一律 full smoke。
func TestTrustedSmokeOwnerScopesRemainCorrect(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		path      string
		quality   string
		platform  string
		container string
	}{
		// 容器 worker 与其 registry 拥有 container 能力，必须跑 full smoke。
		{".github/workflows/container-smoke.yml", "true", "true", "true"},
		{"ci/platforms.json", "true", "true", "true"},
		{"tools/platformmatrix/main.go", "true", "true", "true"},
		// platform worker 只拥有 platform smoke，不涉及容器。
		{".github/workflows/platform-smoke.yml", "true", "true", "false"},
		// 分类策略文件只决定“是否运行”；R6 已让它从 trusted base 执行，
		// 因此 PR 无法用它影响自己的判定，保持 Quality-only 即可。
		{"scripts/classify-change-scope.sh", "true", "false", "false"},
		// 纯文档路径不触发任何 gate。
		{"README.md", "false", "false", "false"},
	} {
		scope := runScopeClassifier(t, test.path)
		for key, want := range map[string]string{
			"quality_required":   test.quality,
			"platform_required":  test.platform,
			"container_required": test.container,
		} {
			if scope[key] != want {
				t.Errorf("changing %s: %s = %q, want %q (scope=%v)", test.path, key, scope[key], want, scope)
			}
		}
	}
}
