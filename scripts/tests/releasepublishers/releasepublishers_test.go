// Package releasepublishers_test 锁住 §14–§24 的 Release/publisher 契约。
package releasepublishers_test

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

func readWorkflow(t *testing.T, root, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, ".github", "workflows", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}

// TestReleasePreparesImmutableHandoff 覆盖 §14/§15：preparation 阶段固化
// build-once 产物的 identity 与 checksum，publish 阶段只校验后复用。
func TestReleasePreparesImmutableHandoff(t *testing.T) {
	t.Parallel()

	body := readWorkflow(t, repositoryRoot(t), "release.yml")
	for _, want := range []string{
		"write-handoff",
		"--output release/release-handoff.json",
		"verify-handoff-set",
		"release/release-handoff.json",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("release.yml must prepare and verify an immutable handoff (%q missing)", want)
		}
	}
	// build once：生产归档与容器内二进制各构建一次，不存在第二遍生产重建。
	if got := strings.Count(body, "sh scripts/build-platform.sh"); got != 2 {
		t.Errorf("release.yml must build exactly twice (production archive + container binary), got %d", got)
	}
	if got := strings.Count(body, "docker build"); got != 1 {
		t.Errorf("release.yml must run docker build exactly once, got %d", got)
	}
}

// TestReleaseNoLongerPublishesHomebrewInline 覆盖 §16.2/§20：Homebrew 是独立
// publisher，release.yml 不得再内联渲染、验证或部署 formula。
func TestReleaseNoLongerPublishesHomebrewInline(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	body := readWorkflow(t, root, "release.yml")
	for _, forbidden := range []string{
		"render_homebrew_formula",
		"verify_homebrew_formula",
		"deploy_homebrew_tap",
		"homebrewformula render",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("release.yml must not publish Homebrew inline (%q found)", forbidden)
		}
	}

	homebrew := readWorkflow(t, root, "publish-homebrew.yml")
	for _, want := range []string{
		"workflow_run:",
		"release_run_id:",
		"release_run_id must be a positive decimal number",
		"verify-handoff-set",
		"--section production",
		"--run-head-sha",
		"homebrewformula render",
		"HOMEBREW_TAP_DEPLOY_KEY",
	} {
		if !strings.Contains(homebrew, want) {
			t.Errorf("publish-homebrew.yml must own Homebrew publication (%q missing)", want)
		}
	}
}

// TestPublishersAcceptOnlyReleaseRunID 覆盖 §17：手动恢复只接受 release_run_id。
func TestPublishersAcceptOnlyReleaseRunID(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, ".github", "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	var publishers []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "publish-") && strings.HasSuffix(entry.Name(), ".yml") {
			publishers = append(publishers, entry.Name())
		}
	}
	if len(publishers) < 4 {
		t.Fatalf("expected at least 4 independent publishers, found %d: %v", len(publishers), publishers)
	}
	for _, name := range publishers {
		body := readWorkflow(t, root, name)
		if !strings.Contains(body, "workflow_run:") {
			t.Errorf("%s must react to the completed Release run", name)
		}
		if !strings.Contains(body, "release_run_id:") {
			t.Errorf("%s must accept release_run_id for recovery", name)
		}
		if strings.Contains(body, "inputs.tag") || strings.Contains(body, "inputs.release_tag") ||
			strings.Contains(body, "latest successful run") || strings.Contains(body, "latest release") {
			t.Errorf("%s must not accept fuzzy recovery inputs", name)
		}
	}
}

// TestPublishersNeverRebuild 覆盖 §19：publisher 只做 download/verify/publish。
func TestPublishersNeverRebuild(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	entries, _ := os.ReadDir(filepath.Join(root, ".github", "workflows"))
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "publish-") || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		body := readWorkflow(t, root, entry.Name())
		for _, forbidden := range []string{"go build", "docker build", "build-platform.sh", "build-release.sh", "package-release"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s must not rebuild release artifacts (%q found)", entry.Name(), forbidden)
			}
		}
	}
}

// TestSingleApprovalBoundaryAcrossReleaseAndPublishers 覆盖 §24。
func TestSingleApprovalBoundaryAcrossReleaseAndPublishers(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	release := readWorkflow(t, root, "release.yml")
	if got := strings.Count(release, "environment: release-approval"); got != 1 {
		t.Fatalf("release.yml must declare exactly one release-approval boundary, got %d", got)
	}
	entries, _ := os.ReadDir(filepath.Join(root, ".github", "workflows"))
	for _, entry := range entries {
		body := readWorkflow(t, root, entry.Name())
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "required_reviewers:") || strings.HasPrefix(trimmed, "reviewers:") {
				t.Errorf("%s must not add a second approval boundary", entry.Name())
			}
		}
	}
}
