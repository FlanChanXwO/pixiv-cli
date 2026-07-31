package main

import (
	"fmt"
	"os"
	"strings"
)

// Validate 确保平台 smoke 继续验证已打包的二进制，而不是退化为只做跨平台编译。
func Validate(path string) error {
	workflow, err := readWorkflow(path)
	if err != nil {
		return err
	}
	for _, required := range []string{
		"pull_request:",
		"push:",
		"workflow_dispatch:",
		"permissions: {}",
		"classify_changes:",
		"go run ./scripts/changescope --base \"$BASE_SHA\" --head \"$HEAD_SHA\" --github-output \"$GITHUB_OUTPUT\"",
		"needs: classify_changes",
		"if: ${{ needs.classify_changes.outputs.docs_only != 'true' }}",
		"    name: Packaged binary smoke\n",
		"platform_smoke_gate:",
		"name: Platform smoke gate",
		"if: ${{ always() }}",
		"needs.packaged_binary_smoke.result",
		"macos-15-intel",
		"macos-15",
		"ubuntu-22.04",
		"ubuntu-22.04-arm",
		"windows-2025",
		"windows-11-arm",
		"go run ./scripts/releaseassets package",
		`go run ./scripts/linuxabi --binary "$binary"`,
		"go test ./scripts/installers -count=1",
		"PIXIV_E2E_BINARY=",
		"PIXIV_E2E_EXPECTED_VERSION=",
		"go test ./e2e -run '^TestPixivBinary' -count=1",
	} {
		if !strings.Contains(workflow, required) {
			return fmt.Errorf("platform smoke workflow missing %q", required)
		}
	}
	if strings.Contains(workflow, "name: Packaged binary smoke ${{") {
		return fmt.Errorf("platform smoke matrix job name must not expose GitHub expression placeholders")
	}
	for _, forbidden := range []string{
		"secrets.",
		"environment:",
		"PIXIV_E2E_REAL_API=1",
		"PIXIV_E2E_WEB_API=1",
	} {
		if strings.Contains(workflow, forbidden) {
			return fmt.Errorf("platform smoke workflow must not contain %q", forbidden)
		}
	}
	return validatePinnedActions(workflow)
}

// ValidateQuality 锁定常规 Linux 门禁，避免 PR 检查被缩减为只编译或只跑单元测试。
func ValidateQuality(path string) error {
	workflow, err := readWorkflow(path)
	if err != nil {
		return err
	}
	for _, required := range []string{
		"pull_request:",
		"push:",
		"workflow_dispatch:",
		"permissions: {}",
		"classify_changes:",
		"go run ./scripts/changescope --base \"$BASE_SHA\" --head \"$HEAD_SHA\" --github-output \"$GITHUB_OUTPUT\"",
		"needs: classify_changes",
		"go test ./scripts/documentation -count=1",
		"if: ${{ needs.classify_changes.outputs.docs_only == 'true' }}",
		"if: ${{ needs.classify_changes.outputs.docs_only != 'true' }}",
		"go test ./... -count=1",
		"go test -race ./... -count=1",
		"go vet ./...",
		"sh scripts/build.sh",
		"sh scripts/test-package-release.sh",
		"sh scripts/test-release-workflow.sh",
		"go test ./scripts/platformsmokeworkflow -count=1",
		"pre_commit run --all-files",
		"windows_login_handler:",
		"name: Windows login handler contracts",
		"CC: clang -fuse-ld=lld",
		"go test ./internal/cli ./internal/cli/loginhelper -count=1",
	} {
		if !strings.Contains(workflow, required) {
			return fmt.Errorf("quality workflow missing %q", required)
		}
	}
	for _, forbidden := range []string{"secrets.", "environment:", "PIXIV_E2E_REAL_API=1", "PIXIV_E2E_WEB_API=1"} {
		if strings.Contains(workflow, forbidden) {
			return fmt.Errorf("quality workflow must not contain %q", forbidden)
		}
	}
	return validatePinnedActions(workflow)
}

func readWorkflow(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func validatePinnedActions(workflow string) error {
	for _, line := range strings.Split(workflow, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "uses: actions/") {
			continue
		}
		if at := strings.LastIndex(trimmed, "@"); at < 0 || len(trimmed[at+1:]) != 40 {
			return fmt.Errorf("workflow action is not pinned by full SHA: %q", trimmed)
		}
	}
	return nil
}
