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

// TestPreparedReleaseArtifactConsumersUsePreservedPaths 锁定 upload-artifact
// 对多路径 artifact 保留相对目录的契约：下载到目标目录后，checksums 与
// handoff 仍分别位于 dist/ 与 release/ 下，所有消费者必须按该布局读取。
func TestPreparedReleaseArtifactConsumersUsePreservedPaths(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	release := readWorkflow(t, root, "release.yml")
	for _, want := range []string{
		"dist/checksums.txt",
		"release/release-handoff.json",
		"done < prepared-release/dist/checksums.txt",
	} {
		if !strings.Contains(release, want) {
			t.Errorf("release.yml must preserve and consume prepared artifact path %q", want)
		}
	}

	for name, wants := range map[string][]string{
		"publish-homebrew.yml": {
			"name: verified-release-checksums",
			"--checksums prepared/dist/checksums.txt",
			"--checksums published-release/checksums.txt",
			"--handoff prepared/release/release-handoff.json",
		},
		"publish-dockerhub.yml": {
			"--handoff prepared/release/release-handoff.json",
			"--dist-dir prepared/dist",
		},
		"publish-clawhub.yml": {
			"--handoff prepared/release/release-handoff.json",
			"jq -r '.tag' prepared/release/release-handoff.json",
		},
		"publish-skillhub.yml": {
			"--handoff prepared/release/release-handoff.json",
			"jq -r '.tag' prepared/release/release-handoff.json",
		},
	} {
		body := readWorkflow(t, root, name)
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s must consume the prepared artifact using preserved path %q", name, want)
			}
		}
	}
}

// TestPublishersCheckoutBeforePreparedHandoffConsumption 锁定 publisher 的工作区顺序：
// checkout 会清理未跟踪文件，而 handoff 校验又依赖仓库内的 Go 工具，因此必须先
// checkout/setup-go，再下载并消费 prepared handoff。
func TestPublishersCheckoutBeforePreparedHandoffConsumption(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, name := range []string{"publish-clawhub.yml", "publish-skillhub.yml", "publish-dockerhub.yml"} {
		body := readWorkflow(t, root, name)
		checkout := strings.Index(body, "actions/checkout@")
		setupGo := strings.Index(body, "actions/setup-go@")
		download := strings.Index(body, "name: Download the prepared handoff")
		if checkout < 0 || setupGo < 0 || download < 0 {
			t.Fatalf("%s must contain checkout, setup-go, and prepared handoff download", name)
		}
		if checkout > download || setupGo > download {
			t.Errorf("%s must checkout and setup Go before downloading the prepared handoff", name)
		}
	}
}

// TestHomebrewLinuxVerificationAcceptsGeneratedVerifyFormula 锁定 Linux Homebrew
// 验证的 staging 契约：宿主机会先生成 *-verify.rb，容器内不能再按生成前的
// 两文件集合做重复断言，否则会在真正执行 brew install 前静默失败。
func TestHomebrewLinuxVerificationAcceptsGeneratedVerifyFormula(t *testing.T) {
	t.Parallel()

	body := readWorkflow(t, repositoryRoot(t), "publish-homebrew.yml")
	if strings.Contains(body, "find /staging-formula -maxdepth 1 -type f -print") {
		t.Fatal("publish-homebrew.yml must not re-check the pre-generation staging file set inside the Linux container")
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

// TestHomebrewHasExactlyOnePublishPath 覆盖 §18/§25.3：Homebrew 只能有一个
// 发布路径。若 prepublish 验证 workflow 也保留 deploy 分支去写同一个 tap，
// 就出现了第二个不经 immutable handoff 校验的发布入口。
func TestHomebrewHasExactlyOnePublishPath(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, ".github", "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	var writers []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}
		body := readWorkflow(t, root, entry.Name())
		// tap 写入的唯一特征是携带 Homebrew 部署密钥并 push 到 tap。
		if strings.Contains(body, "HOMEBREW_TAP_DEPLOY_KEY") {
			writers = append(writers, entry.Name())
		}
	}
	if len(writers) != 1 || writers[0] != "publish-homebrew.yml" {
		t.Fatalf("Homebrew tap must have exactly one publisher (publish-homebrew.yml), got %v", writers)
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

// TestHomebrewDeployIsMonotonic 覆盖 R10/R11：deploy 必须由 trusted
// homebrewrecovery 判定驱动，同版本幂等 no-op，旧版本在写入前 fail closed。
// 判定代码来自默认分支 tip，不随被恢复的 release tag 一起回退。
func TestHomebrewDeployIsMonotonic(t *testing.T) {
	t.Parallel()

	body := readWorkflow(t, repositoryRoot(t), "publish-homebrew.yml")
	for _, want := range []string{
		"scripts/cmd/homebrewrecovery",
		"--current \"$tap_dir/Formula/$formula_name.rb\"",
		"--requested \"staging-formula/$formula_name.rb\"",
		"steps.deploy.outputs.action == 'install'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("publish-homebrew.yml must gate the tap write on the trusted recovery decision (%q missing)", want)
		}
	}

	// 只有安装分支可以拿到 deploy key 并 push；no-op 分支不得触达 tap。
	prepare := strings.Index(body, "Prepare an exact one-formula tap commit")
	push := strings.Index(body, "Push the verified formula with the protected deploy key")
	decision := strings.Index(body, "Decide whether the tap may accept this formula")
	if decision < 0 || prepare < 0 || push < 0 {
		t.Fatal("publish-homebrew.yml must decide, prepare, then push")
	}
	if !(decision < prepare && decision < push) {
		t.Error("publish-homebrew.yml must run the recovery decision before any tap write step")
	}
	for _, gate := range []string{
		"if: ${{ steps.deploy.outputs.action == 'install' }}",
	} {
		if got := strings.Count(body, gate); got != 2 {
			t.Errorf("tap commit and push must both be gated by %q, found %d", gate, got)
		}
	}

	// 判定必须在 checkout protected default-branch tip 之后运行。
	if strings.Contains(body, "ref: ${{ needs.publish_homebrew.outputs.commit_sha }}") {
		t.Error("publish-homebrew.yml must not run the deploy decision from the release commit")
	}
	if !strings.Contains(body, "ref: ${{ github.sha }}") {
		t.Error("publish-homebrew.yml deploy must checkout the protected default-branch tip")
	}
}
